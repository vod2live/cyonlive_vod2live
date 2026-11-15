# Runbook de staging e produção

## Objetivos

- RPO alvo: 15 minutos, com backup PostgreSQL frequente e cópia externa.
- RTO alvo: 30 minutos para restaurar banco, secrets, API, Redis e workers.
- Redis é estado derivado; o PostgreSQL é a fonte persistente de identidade, canais, assets e grade.

## Deploy

1. Validar CI verde, migrações reaplicáveis e dump verificável.
2. Aplicar secrets no ambiente sem gravá-los no repositório.
3. Construir e publicar uma imagem única a partir do commit aprovado. A imagem contém `api`, `playout` e `media-worker`; `SERVICE` seleciona explicitamente o processo em runtime. Use o SHA do commit como tag de publicação e não reutilize tags mutáveis:

   ```sh
   COMMIT=$(git rev-parse HEAD)
   IMAGE=ghcr.io/vod2live/cyonlive_vod2live:$COMMIT
   docker build -f deployments/docker/Dockerfile \
     --build-arg VERSION="$COMMIT" \
     --build-arg COMMIT="$COMMIT" \
     --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
     -t "$IMAGE" .
   docker push "$IMAGE"
   ```

   Após o push, obtenha o digest retornado pelo registro e configure `IMAGE_REF=ghcr.io/vod2live/cyonlive_vod2live@sha256:<digest>` no secret manager. O Compose usa o mesmo digest imutável para os três serviços.
4. Executar `docker compose --env-file deployments/staging/.env -f deployments/staging/docker-compose.staging.yml --profile tools run --rm migration-runner` antes de iniciar a nova versão ou trocar o tráfego. O arquivo `.env` deve ser provisionado fora do Git a partir do secret manager; o processo usa `DATABASE_URL`, mantém o advisory lock na mesma conexão e recusa checksums alterados.
5. Subir a nova versão em staging e executar E2E, contrato, load e HA smoke. O override de staging remove as dependências de containers locais; `DATABASE_URL`, `REDIS_ADDR` e `MINIO_ENDPOINT` devem apontar aos serviços externos. O Compose de staging executa a imagem como usuário não-root, com filesystem somente leitura, `no-new-privileges`, capabilities removidas, `tmpfs` limitado e limites explícitos de CPU, memória e PIDs; ajuste os limites por ambiente somente após validar a carga.
6. Fazer deploy rolling em produção.
7. Confirmar `/readyz`, `/version`, métricas e playback assinado.
8. Confirmar no ambiente-alvo que TLS/ingress, política de rede, armazenamento de secrets e limites externos estão ativos; esses controles não são fornecidos pelo Compose e não devem ser presumidos.

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
