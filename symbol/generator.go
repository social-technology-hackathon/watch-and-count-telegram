package symbol

import (
	"math/rand"
	"time"

	"github.com/speps/go-hashids"
)

const (
	alphabet = `АБВГДЕЖИКЛМНОПРСТУФХЦЧШЩЫЭЮЯ123456789()-=%$#@!*[]/\|?✔✖"`
)

func init() {
	rand.Seed(time.Now().Unix())
}

type Generator struct {
	h *hashids.HashID
}

func New(secret string) (*Generator, error) {
	cfg := hashids.NewData()
	cfg.Salt = secret
	cfg.Alphabet = alphabet
	cfg.MinLength = 3
	h, err := hashids.NewWithData(cfg)
	if err != nil {
		return nil, err
	}
	return &Generator{
		h: h,
	}, nil
}

func (g *Generator) Generate() (string, error) {
	// TODO: should be calculated
	someID := rand.Intn(1000)
	return g.h.Encode([]int{someID})
}
