# Email Service (Gmail SMTP)

Serviço interno de envio de emails via Gmail SMTP para os sites Provengo/Voipe.

## Setup do Gmail

1. Criar conta `alerts.provengo@gmail.com`
2. Ativar **2-Step Verification** em [myaccount.google.com/security](https://myaccount.google.com/security)
3. Gerar **App Password** em [myaccount.google.com/apppasswords](https://myaccount.google.com/apppasswords)
4. Copiar a senha de 16 caracteres para o `.env` (campo `SMTP_PASS`, sem espaços)

## Deploy

```bash
# 1. SQL no banco
psql -h 198.18.12.76 -U sender_mail -d sender_mail -f migrations/001_create_emails.sql

# 2. Configurar .env com o App Password
cp .env.example .env
vi .env  # preencher SMTP_PASS

# 3. Build e start
docker compose up -d --build
```

## API

### POST /api/v1/send

```bash
curl -X POST http://localhost:8080/api/v1/send \
  -H "Content-Type: application/json" \
  -d '{
    "origem": "provengo.io",
    "campanha": "contato",
    "email_src": "cliente@example.com",
    "name": "João Silva",
    "assunto": "Dúvida sobre o produto",
    "corpo": "Olá, gostaria de saber mais sobre os planos."
  }'
```

### GET /api/v1/emails
Lista últimos 100 emails.

### GET /health
Health check.

## Variáveis de ambiente

| Variável     | Default                     |
|--------------|-----------------------------|
| PORT         | 8080                        |
| SMTP_HOST    | smtp.gmail.com              |
| SMTP_PORT    | 587                         |
| SMTP_USER    | alerts.provengo@gmail.com   |
| SMTP_PASS    | (App Password - obrigatório)|
| DEST_EMAIL   | alerts.provengo@gmail.com   |
| DATABASE_URL | (obrigatório)               |

## Limites Gmail

- Conta gratuita: 500 emails/dia
- Google Workspace: 2.000 emails/dia
