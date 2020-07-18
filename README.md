# Vybar

TG bot

## How to run

Create docker-compose.local.yml file with next content

```yaml
version: '3.7'

services:
  telegram:
    environment:
      - VERBOSE=1
      - TELEGRAM_TOKEN=<bot token>
```

```bash
docker-compose -f ./docker-compose.yml -f ./docker-compose.local.yml up --build
```

You can get token from [@BotFather](https://t.me/botfather)

## Enviroment varables

```bash
VERBOSE=1  # turns verbose logging on
TELEGRAM_TOKEN=  # token
STORAGE_TYPE=file  # enum, possible values - file, spaces, s3
STORAGE_PATH=  # required fo STORAGE_TYPE=file, base path, where media files will stored
STORAGE_KEY=  # required for s3 or spaces storage type, access key for storage
STORAGE_SECRET=  # required for s3 or spaces storage type, access secret key for storage
STORAGE_ENDPOINT=  # required for spaces, endpoint for your bucket
STORAGE_REGION=  # required fo s3, region for your bucket
STORAGE_BUCKET=  # required for s3 or spaces storage type, bucket name
```
