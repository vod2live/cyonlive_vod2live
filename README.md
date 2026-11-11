# VOD2Live Backend

Backend modular para montar canais lineares usando somente fontes HLS (`master.m3u8` ou media playlist). O projeto não faz transcodificação: valida manifests, persiste a grade, mantém o estado do playout e publica manifests HLS lineares. Quando a entrada é uma media playlist direta, o canal opera como uma única variante.

## Componentes

- `api`: REST para canais, assets e grade.
- `validator`: leitura e validação de URLs HLS.
- `scheduler`: resolução da programação publicada.
- `playout`: relógio e estado por canal.
- `manifest`: montagem da janela HLS de saída.
- PostgreSQL: configuração, catálogo e grade.
- Redis: locks, cache e estado quente.

O Redis mantém cache curto dos manifests (`master.m3u8` e media playlists), estado quente por canal e locks distribuídos para impedir que dois workers controlem o mesmo playout. O PostgreSQL continua sendo a fonte permanente da configuração e da grade.

O worker separado `cmd/playout` consulta canais com status `running`, adquire o lock Redis, calcula a posição atual na programação e atualiza o estado quente do canal a cada 5 segundos.

## Executar

```bash
docker compose up --build
curl http://localhost:8080/healthz
# readiness: PostgreSQL + Redis
curl http://localhost:8080/readyz
# smoke test
./tests/integration/smoke.sh
```

## Entrada de asset

```json
{
  "title": "Episódio 01",
  "hls_url": "https://cdn.example.com/episode-01/master.m3u8"
}
```

## Endpoints iniciais

- `GET /healthz`
- `GET /readyz`
- `POST /v1/assets`
- `GET /v1/assets/{id}`
- `POST /v1/assets/{id}/validate`
- `POST /v1/channels`
- `GET /v1/channels`
- `GET /v1/channels/{id}`
- `POST /v1/channels/{id}/start`
- `POST /v1/channels/{id}/stop`
- `GET /v1/channels/{id}/status`
- `POST /v1/channels/{id}/schedule`
- `GET /v1/channels/{id}/schedule`
- `PATCH /v1/schedule/{id}`
- `DELETE /v1/schedule/{id}`
- `GET /v1/channels/{id}/master.m3u8`
- `GET /v1/channels/{id}/media/{variant}.m3u8`

## Limites atuais do esqueleto

A montagem real de manifests e a persistência dos repositórios estão isoladas por interfaces para implementação incremental. Não são aceitos MP4, MPEG-TS ou upload de arquivos.

A especificação de integração está em [`openapi.yaml`](./openapi.yaml).

## Fase 1 concluída

A Fase 1 entrega um MVP executável com persistência PostgreSQL, cache e locks Redis, validação de master/media playlists HLS, grade com CRUD e detecção de conflitos, Manifest Builder, worker contínuo de playout, endpoints de operação, Docker Compose e pipeline CI. O fluxo completo pode ser validado com `docker compose up --build -d` seguido de `./tests/integration/e2e.sh`.

O CI executa `go test ./...`, constrói os serviços Docker e roda o fluxo end-to-end com PostgreSQL, Redis e uma fixture HLS local. A Fase 2 deve tratar autenticação, segurança de URLs, SSAI/SCTE-35, observabilidade avançada e deploy de produção.

## Fase 2 — segurança inicial

Os endpoints de negócio `/v1/*` agora exigem `Authorization: Bearer <access_token>`. `API_KEY` permanece somente para o bootstrap inicial do primeiro administrador; não é aceito nas rotas de conteúdo. `/healthz`, `/readyz`, `/metrics` e `/version` permanecem públicos para operação.

## Fase 3 — observabilidade inicial

O endpoint público `/metrics` expõe métricas no formato Prometheus: requisições HTTP, erros, falhas de autenticação, solicitações de manifests e estado de readiness. Em produção, restrinja o acesso ao endpoint por rede, ingress ou autenticação do próprio monitoramento.

## Fase 4 — hardening de origens HLS

O validador HLS bloqueia por padrão origens `localhost`, loopback, IPs privados, link-local, multicast e não especificados, reduzindo risco de SSRF. Para testes locais com o serviço `hls-fixture`, o Compose define `ALLOW_PRIVATE_HLS=true`; em produção, mantenha essa variável como `false`.

## Fase 5 — limites de ingestão HLS

Manifests HLS têm limite de tamanho configurável por `HLS_MAX_MANIFEST_BYTES` (8 MiB por padrão), timeout por `HLS_REQUEST_TIMEOUT_SECONDS` (15 segundos por padrão) e no máximo três redirecionamentos HTTP. Respostas maiores ou excessivamente redirecionadas são rejeitadas antes do processamento.

## Fase 6 — proteção operacional da API

Os endpoints `/v1/*` possuem rate limiting por IP, com limite padrão de 120 requisições por minuto configurável por `RATE_LIMIT_PER_MINUTE`. Quando excedido, retornam `429` com `Retry-After`. A API também envia headers básicos de segurança, incluindo `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` e `Permissions-Policy`.

## Fase 7 — rastreabilidade operacional

Cada resposta HTTP recebe `X-Request-ID`. Se o cliente enviar esse header, o mesmo valor é preservado; caso contrário, a API gera um identificador aleatório. Cada requisição é registrada em JSON via `slog`, incluindo método, caminho, status, duração e request ID, facilitando correlação em agregadores de logs.

## Fase 8 — lifecycle seguro

API e worker de playout tratam `SIGTERM` e `SIGINT`. A API deixa de aceitar novas conexões e aguarda até 10 segundos para encerrar requisições em andamento; o worker cancela o loop de playout e libera recursos. O Compose reserva 15 segundos de `stop_grace_period` para o processo concluir o shutdown.

## Fase 9 — resiliência de dependências

PostgreSQL e Redis usam retry com backoff exponencial na inicialização. O processo tenta até oito vezes, aumenta o intervalo entre tentativas até cinco segundos e respeita cancelamento por sinal, evitando falha prematura durante o boot ordenado dos serviços.

## Fase 10 — contrato HTTP

Os testes de integração criam o administrador por bootstrap, fazem login bearer e verificam o acesso autenticado. O contrato valida request ID, headers de segurança, rejeição de chamadas anônimas e da API key antiga, acesso autenticado a canais e métricas Prometheus.

## Fase 11 — configuração segura por ambiente

`APP_ENV=production` ativa validações fail-fast: `API_KEY` de bootstrap é obrigatório; `PLAYBACK_TOKEN_SECRET` e `AUTH_TOKEN_SECRET` precisam ser diferentes entre si, únicos para a instalação e ter pelo menos 32 bytes; `ALLOW_PRIVATE_HLS` precisa estar desativado e os TTLs/limites devem ser positivos. A aplicação encerra antes de abrir portas quando a configuração é inválida.

## Fase 12 — diagnóstico e versão

O endpoint público `GET /version` retorna versão, commit e horário de build. O Dockerfile aceita os argumentos `VERSION`, `COMMIT` e `BUILD_TIME`, permitindo que o CI injete a identidade exata do artefato publicado.

## Fase 13 — autenticação de playback HLS

O endpoint administrativo `POST /v1/channels/{id}/playback` emite, mediante bearer token com papel `administrador` ou `operador`, uma URL temporária específica para o canal. A resposta inclui `playback_url` relativo e `expires_at`; o player acessa o link sem receber nem expor as credenciais de API. O master, as variantes e cada segmento usam assinaturas HMAC-SHA256 vinculadas ao canal e ao recurso. Tokens ausentes ou inválidos recebem `403`; tokens expirados recebem `410 Gone`.

O master e as media playlists são servidos publicamente em `/playback/{channelID}/master.m3u8` e `/playback/{channelID}/media/{variant}.m3u8`. As URLs dos segmentos são reescritas para `/playback/{channelID}/segment`, que valida a assinatura e faz proxy da origem HLS; as respostas de playback usam `Cache-Control: private, no-store` e `Referrer-Policy: no-referrer`. O proxy restringe URLs a HTTP(S), bloqueia endereços privados quando `ALLOW_PRIVATE_HLS=false` e não segue redirecionamentos.

Configure `PLAYBACK_TOKEN_SECRET` como segredo aleatório com pelo menos 32 bytes e `PLAYBACK_TOKEN_TTL_SECONDS` (padrão: 600). O Compose define um segredo fixo apenas para desenvolvimento local. Em `APP_ENV=production`, a inicialização falha se o segredo de desenvolvimento for usado, se o segredo tiver menos de 32 bytes ou se `ALLOW_PRIVATE_HLS` estiver habilitado. Compartilhe as URLs emitidas como credenciais temporárias e nunca as registre em logs ou referers.

Exemplo de emissão:

```sh
curl -X POST -H 'Authorization: Bearer <ACCESS_TOKEN>' \
  http://localhost:8080/v1/channels/<CHANNEL_ID>/playback
```

O contrato está descrito em [`openapi.yaml`](./openapi.yaml) e os testes de assinatura, expiração, escopo por canal/recurso, reescrita de manifest e proxy de segmentos estão em `internal/playback`.

## Fase 14 — usuários, organizações e RBAC

A migração `migrations/002_phase14_identity_tenant.sql` cria organizações, usuários com senha bcrypt, associação usuário-organização, equipes, membros de equipe e trilha de auditoria. Ela transfere os canais, assets e itens da grade existentes para a organização legada; as chaves estrangeiras compostas impedem que um item de grade associe canal e asset de organizações diferentes.

Em uma instalação existente, aplique a migração antes de atualizar a API. No Compose, execute `docker compose exec -T postgres psql -U vod2live -d vod2live < migrations/002_phase14_identity_tenant.sql`; em instalações próprias, use a credencial de migração correspondente. Em banco novo, o Compose aplica os arquivos em `migrations/` durante a inicialização do volume. Para o primeiro acesso, use `POST /v1/auth/bootstrap` com `X-API-Key: <API_KEY>`, email, senha de pelo menos 12 caracteres e `organization_name`. Esse endpoint só funciona quando ainda não existe usuário; a chave antiga não autentica mais operações de negócio. Depois, use `POST /v1/auth/login` para receber um token bearer de 15 minutos. Se o usuário pertencer a várias organizações, informe `organization_id` no login.

Papéis suportados: `administrador`, `operador`, `editor` e `visualizador`. Administradores gerenciam membros/papéis/equipes; podem criar usuários novos (senha obrigatória) ou associar usuários já cadastrados a outra organização (sem alterar sua senha). Editores administram assets, canais e grade; operadores iniciam/param canais e emitem links de playback; visualizadores têm acesso de leitura. A associação e o papel são recarregados do banco a cada chamada, então remoção e alteração de papel têm efeito imediato. As operações de escrita autenticadas registram ator, organização, ação, recurso, status HTTP e request ID em `audit_events`.

As consultas HTTP de assets, canais e grade usam o tenant do token. As equipes e associações também são limitadas à organização, e o banco reforça o isolamento com chaves estrangeiras compostas. `AUTH_TOKEN_SECRET` deve ser um segredo aleatório e independente do segredo de playback; o Compose fornece um valor apenas para desenvolvimento. O E2E valida criação de organização/usuários, RBAC, isolamento entre tenants, emissão de link assinado e rejeição de playback sem token. Para validar a suíte de integração em uma stack limpa, execute `docker compose down -v` e depois `docker compose up --build -d`, `./tests/integration/e2e.sh` e `./tests/integration/contract.sh`.

## Fase 15 — observabilidade avançada

Além dos contadores básicos, `/metrics` expõe contagem de requisições por status HTTP e histograma de latência com buckets fixos. Não são usados labels livres de rota, tenant ou usuário, evitando cardinalidade explosiva no Prometheus.

## Fase 16 — entrega CDN-friendly

Manifests de playback agora enviam `ETag` SHA-256 e respondem `304 Not Modified` quando o cliente reapresenta a mesma versão. A origem CORS é configurável por `CORS_ALLOWED_ORIGIN`; o Compose usa `*` apenas para desenvolvimento. Em produção, informe a origem exata do player e mantenha o cache privado dos manifests assinados.

## Fase 17 — alta disponibilidade do playout

Cada ciclo de playout usa lease Redis com TTL, renovação antes da publicação e fencing token monotônico por canal. A escrita do estado é atômica e só ocorre se o worker ainda possuir o lease correspondente, impedindo que um processo antigo publique estado depois de perder a liderança.

## Fase 18 — continuidade de programação

Quando `loop_enabled=true`, o Manifest Builder calcula a duração real da grade a partir dos `ends_at` ou das durações dos assets e reposiciona o relógio no início do ciclo após o último item. Isso evita que um canal linear fique sem segmentos ao final da programação. A próxima extensão da fase é adicionar eventos comerciais e SCTE-35 sobre essa base.

## Fase 19 — metadados HLS de eventos

O parser e o serializador preservam tags `#EXT-X-DATERANGE` nas media playlists, mantendo marcadores de eventos durante o processamento. Essa base permite adicionar cues comerciais e SCTE-35 sem descartar metadados HLS existentes.

## Fase 20 — operação de banco e recuperação

O projeto fornece backup PostgreSQL em formato customizado, verificação do catálogo do dump e restore explícito protegido por confirmação. Com a stack ativa, use `make db-backup`, valide com `make db-verify-backup BACKUP_FILE=backups/arquivo.dump` e só execute `make db-restore BACKUP_FILE=...` após confirmar que a operação destrutiva é desejada. O CI também gera e valida um dump temporário em cada execução integrada.

## Fase 21 — teste de carga operacional

O script `scripts/load-test.sh` executa requisições concorrentes contra `/healthz`, `/readyz` e `/version`, com `REQUESTS` e `CONCURRENCY` configuráveis. O CI roda 120 requisições com concorrência 8 e falha quando qualquer endpoint retorna status diferente de 200. Execute localmente com `REQUESTS=500 CONCURRENCY=16 make load-test`.

## Fase 22 — migrações reaplicáveis

As constraints da migração multi-tenant usam blocos idempotentes e podem ser reaplicadas sem erro. O script `scripts/verify-migrations.sh` executa novamente todos os SQLs com `ON_ERROR_STOP=1`; o CI e o comando `make verify-migrations` usam esse gate para detectar migrações incompatíveis com upgrades.

## Pré-produção operacional

O CI também valida a configuração do Compose, executa o smoke de lease/fencing Redis e faz carga concorrente sobre uma URL de playback emitida pelo E2E. Os comandos locais equivalentes são `make ha-smoke` e `PLAYBACK_URL=... make playback-load-test`.

O template de staging está em [`deployments/staging/.env.example`](deployments/staging/.env.example), o Compose de imagens imutáveis em [`deployments/staging/docker-compose.staging.yml`](deployments/staging/docker-compose.staging.yml), o runbook de RPO/RTO e rollback em [`deployments/staging/RUNBOOK.md`](deployments/staging/RUNBOOK.md) e a estratégia CDN em [`deployments/staging/CDN.md`](deployments/staging/CDN.md). Não coloque segredos nesses arquivos; injete-os pelo ambiente ou pelo secret manager.
