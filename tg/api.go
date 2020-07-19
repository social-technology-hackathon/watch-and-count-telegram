package tg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"vybar/tg/file"
	"vybar/tg/message"
	"vybar/tg/user"

	"github.com/sirupsen/logrus"
)

var (
	tgBaseURL *url.URL
)

func init() {
	u, err := url.Parse("https://api.telegram.org")
	if err != nil {
		panic(err)
	}
	tgBaseURL = u
}

type Logger interface {
	Debugf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Infof(format string, args ...interface{})
}

type API struct {
	token      string
	httpClient *http.Client
	logger     Logger
	botData    BotUser
}

type Option func(*API)

func HTTPClient(c *http.Client) Option {
	return func(a *API) {
		a.httpClient = c
	}
}

func WithLogger(logger Logger) Option {
	return func(a *API) {
		a.logger = logger
	}
}

func New(token string, options ...Option) (*API, error) {
	api := API{
		token:      token,
		httpClient: http.DefaultClient,
		logger:     logrus.StandardLogger(),
	}

	for _, opt := range options {
		opt(&api)
	}

	botData, err := api.GetMe()
	if err != nil {
		return nil, err
	}
	api.botData = *botData
	return &api, err
}

type BotUser struct {
	user.User
	CanJoinGroups           bool `json:"can_join_groups"`
	CanReadAllGroupMessages bool `json:"can_read_all_group_messages"`
	SupportsInlineQueries   bool `json:"supports_inline_queries"`
}

type tgFile struct {
	r *http.Response
}

func (f *tgFile) Read(b []byte) (int, error) {
	return f.r.Body.Read(b)
}

func (f *tgFile) Close() error {
	io.Copy(ioutil.Discard, f.r.Body)
	return f.r.Body.Close()
}

var (
	_ io.ReadCloser = (*tgFile)(nil)
)

func (api *API) newFileRequest(ctx context.Context, filePath string) (*http.Request, error) {
	u, err := url.Parse(filePath)
	if err != nil {
		return nil, err
	}

	u.Path = path.Join("file", fmt.Sprintf("bot%s", api.token), u.Path)
	u = tgBaseURL.ResolveReference(u)

	api.logger.Debugf("tg: GET -> %s", strings.ReplaceAll(u.String(), api.token, "*****"))

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	return req, err
}

func (api *API) newRequest(ctx context.Context, method, relURL string, body interface{}) (*http.Request, error) {
	var bodyReader io.Reader
	headers := make(http.Header)
	headers.Set("Accept", "application/json")

	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, err
		}
		bodyReader = &buf
		api.logger.Debugf("tg: body is %s", buf.String())
		headers.Set("Content-Type", "application/json")
	}

	u, err := url.Parse(relURL)
	if err != nil {
		return nil, err
	}

	u.Path = path.Join(fmt.Sprintf("bot%s", api.token), u.Path)
	u = tgBaseURL.ResolveReference(u)

	api.logger.Debugf("tg: %s -> %s", method, strings.ReplaceAll(u.String(), api.token, "*****"))

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	api.logger.Debugf("tg: headers: %+v", headers)
	return req, nil
}

type tgResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
}

func (api *API) do(r *http.Request, dst interface{}) error {
	resp, err := api.httpClient.Do(r)
	if err != nil {
		return err
	}

	// DEBUG
	var debugBuf bytes.Buffer
	rdr := io.TeeReader(resp.Body, &debugBuf)
	defer func() {
		if debugBuf.Len() > 0 {
			api.logger.Debugf("tg response: %s", debugBuf.String())
		}
	}()

	defer func() {
		io.Copy(ioutil.Discard, rdr)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tg: failed to request telegram api: returned not 200 OK")
	}

	if dst != nil {
		var rsp tgResponse
		if err := json.NewDecoder(rdr).Decode(&rsp); err != nil {
			return err
		}
		if !rsp.OK {
			return fmt.Errorf("tg: response not ok")
		}
		if err := json.Unmarshal(rsp.Result, dst); err != nil {
			return err
		}
	}

	return nil
}

func (api *API) GetMe() (*BotUser, error) {
	req, err := api.newRequest(context.Background(), "GET", "getMe", nil)
	if err != nil {
		return nil, err
	}

	var resp BotUser
	if err := api.do(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (api *API) Username() string {
	return *api.botData.Username
}

type Update struct {
	ID      int              `json:"update_id"`
	Message *message.Message `json:"message"`
}

func (api *API) GetUpdatesContext(ctx context.Context, offset int) ([]*Update, error) {
	prms := make(url.Values)
	// prms.Add("timeout", "600")
	if offset != 0 {
		prms.Add("offset", strconv.Itoa(offset))
	}
	u := &url.URL{
		Path:     "getUpdates",
		RawQuery: prms.Encode(),
	}
	req, err := api.newRequest(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	var res []*Update
	if err := api.do(req, &res); err != nil {
		return nil, err
	}

	return res, nil
}

func (api *API) GetUpdatesChan(ctx context.Context, offset int) (<-chan Update, error) {
	result := make(chan Update)

	go func() {
		defer close(result)
		for {
			upds, err := api.GetUpdatesContext(ctx, offset)
			if err != nil {
				api.logger.Errorf("tg: %s", err.Error())
				return
			}

			for _, upd := range upds {
				offset = upd.ID
				result <- *upd
			}
			if len(upds) > 0 {
				offset++
			}

			select {
			case <-ctx.Done():
				api.logger.Infof("tg: stop updates worker: %s", ctx.Err().Error())
				return
			default:
				continue
			}
		}
	}()

	return result, nil
}

func (api *API) SendMessage(msg *message.Message) (*message.Message, error) {
	req := struct {
		ChatID           int64           `json:"chat_id"`
		Text             string          `json:"text"`
		ReplyToMessageID int             `json:"reply_to_message_id"`
		ReplyMarkup      json.RawMessage `json:"reply_markup,omitempty"`
		ParseMode        string          `json:"parse_mode"`
	}{
		ChatID: msg.Chat.ID,
	}

	if msg.Markdown {
		req.ParseMode = "MarkdownV2"
	}

	if msg.Text != nil {
		req.Text = *msg.Text
	}

	if msg.ReplyToMessage != nil {
		req.ReplyToMessageID = msg.ReplyToMessage.ID
	}

	if msg.ReplyMarkup != nil {
		d, err := msg.ReplyMarkup.Serialize()
		if err != nil {
			return nil, err
		}
		req.ReplyMarkup = d
	}

	r, err := api.newRequest(context.Background(), "POST", "sendMessage", &req)
	if err != nil {
		return nil, err
	}

	var resp message.Message
	if err := api.do(r, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (api *API) GetFile(fileID string) (*file.File, error) {
	req := struct {
		FileID string `json:"file_id"`
	}{
		FileID: fileID,
	}

	r, err := api.newRequest(context.Background(), "POST", "getFile", &req)
	if err != nil {
		return nil, err
	}

	var resp file.File
	if err := api.do(r, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (api *API) GetFD(fileID string) (io.ReadCloser, error) {
	f, err := api.GetFile(fileID)
	if err != nil {
		return nil, err
	}

	if f.FilePath == nil {
		return nil, fmt.Errorf("tg: telegram servers does not return file_path")
	}

	req, err := api.newFileRequest(context.Background(), *f.FilePath)
	if err != nil {
		return nil, err
	}

	resp, err := api.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return &tgFile{resp}, nil
}
