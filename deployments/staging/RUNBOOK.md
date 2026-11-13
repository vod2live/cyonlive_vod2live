# Runbook de staging e produção

## Objetivos

- RPO alvo: 15 minutos, com backup PostgreSQL frequente e cópia externa.
- RTO alvo: 30 minutos para restaurar banco, secrets, API, Redis e workers.
- Redis é estado derivado; o PostgreSQL é a fonte persistente de identidade, canais, assets e grade.

## Deploy

1. Validar CI verde, migrações reaplicáveis e dump verificável.
2. Aplicar secrets no ambiente sem gravá-los no repositório.
3. Executar `docker compose --env-file deployments/staging/.env -f deployments/staging/docker-compose.staging.yml --profile tools run --rm migration-runner` antes de iniciar a nova versão ou trocar o tráfego. O arquivo `.env` deve ser provisionado fora do Git a partir do secret manager; o processo usa `DATABASE_URL`, mantém o advisory lock na mesma conexão e recusa checksums alterados.
4. Subir a nova versão em staging e executar E2E, contrato, load e HA smoke.
5. Fazer deploy rolling em produção.
6. Confirmar `/readyz`, `/version`, métricas e playback assinado.

## Rollback

1. Parar a promoção da versão nova.
2. Reverter para a imagem anterior pelo digest imutável.
3. Não reverter migrações automaticamente; usar procedimento SQL compatível e backup antes da alteração.
4. Confirmar readiness, canais ativos e playback.
5. Registrar incidente, commit, digest e horário.

## Recuperação

1. Provisionar PostgreSQL limpo.
2. Restaurar o último dump validado com `CONFIRM_RESTORE=YES make db-restore BACKUP_FILE=...`.
3. Aplicar migrações reaplicáveis.
4. Subir API e playout com secrets originais ou rotacionados.
5. Executar E2E e smoke load antes de liberar tráfego.
