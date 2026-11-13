# VOD2Live Backend

Backend profissional para transformar ativos VOD HLS em canais lineares HLS. O sistema monta a grade, calcula a posição temporal do playout e publica manifests dinâmicos de saída. Não há transcodificação: os conteúdos de entrada precisam ser disponibilizados como playlists HLS (`master.m3u8` ou media playlist) e seus segmentos continuam sendo servidos pela origem/CDN do conteúdo.

> **Status:** MVP avançado em pré-produção. O núcleo funcional, segurança inicial, multi-tenancy, playout, observabilidade e operação de banco estão implementados. A validação final em ambiente real ainda depende de infraestrutura com Docker, PostgreSQL, Redis, secret manager, CDN e testes de carga representativos.

## Sumário

- [Visão geral](#visão-geral)
- [Arquitetura](#arquitetura)
- [Fluxo operacional](#fluxo-operacional)
- [Tecnologias](#tecnologias)
- [Pré-requisitos](#pré-requisitos)
- [Execução local](#execução-local)
- [Configuração](#configuração)
- [Autenticação e RBAC](#autenticação-e-rbac)
- [Multi-tenancy](#multi-tenancy)
- [Ingestão HLS e SSRF](#ingestão-hls-e-ssrf)
- [Playback assinado](#playback-assinado)
- [Manifest Builder](#manifest-builder)
- [Alta disponibilidade](#alta-disponibilidade)
- [Observabilidade](#observabilidade)
- [Banco, backup e recuperação](#banco-backup-e-recuperação)
- [Staging e CDN](#staging-e-cdn)
- [Testes e CI](#testes-e-ci)
- [Estrutura do projeto](#estrutura-do-projeto)
- [API](#api)
- [Limitações e próximos passos](#limitações-e-próximos-passos)

## Visão geral

O VOD2Live recebe URLs HLS, valida os manifests, cadastra os ativos, associa esses ativos a canais e persiste uma grade temporal. O worker de playout consulta os canais ativos, adquire liderança distribuída no Redis e mantém o relógio de cada canal. A API monta manifests lineares a partir da posição atual da grade, preservando metadados HLS relevantes e reescrevendo os recursos para playback seguro.

O sistema é adequado para cenários como:

- canais FAST e canais corporativos baseados em VOD;
- programação linear sem transcodificação;
- montagem de grades com loop contínuo;
- distribuição de playback por CDN;
- operação multi-organização com papéis administrativos;
- execução horizontal com múltiplos workers de playout.

## Arquitetura

```text
                    +---------------------+
                    |  Painel/Operador    |
                    +----------+----------+
                               |
                         Bearer Token
                               |
                    +----------v----------+
                    |       API Go        |
                    | REST + Playback HLS |
                    +----+-----------+----+
                         |           |
              +----------v--+     +--v----------+
              | PostgreSQL  |     |    Redis     |
              | fonte       |     | cache/locks  |
              | persistente |     | fencing      |
              +-------------+     +------+-------+
                                         |
                                  +------v-------+
                                  | Playout Go   |
                                  | workers HA   |
                                  +------+-------+
                                         |
                                  Manifest linear
                                         |
                                  +------v-------+
                                  | CDN / Player |
                                  +--------------+

Origem HLS dos ativos ------------------------^
```

### Componentes

- **API (`cmd/api`)**: REST, autenticação, RBAC, canais, ativos, grade, playback e métricas.
- **Playout (`cmd/playout`)**: worker contínuo que controla o relógio dos canais e publica estado quente.
- **PostgreSQL 16**: organizações, usuários, equipes, ativos, canais, grade e auditoria.
- **Redis 7**: cache curto de manifests, leases, locks, fencing tokens e estado quente.
- **Parser HLS (`pkg/hls`)**: leitura e serialização de master/media playlists.
- **Manifest service (`internal/manifest`)**: resolução da grade e construção do HLS linear.
- **Playback (`internal/playback`)**: assinatura HMAC, validação de escopo e proxy de segmentos.

## Fluxo operacional

1. O administrador cria a organização e o primeiro usuário.
2. Um usuário autorizado cadastra uma URL HLS.
3. O backend valida master/media playlist, tamanho, timeout, redirects e origem de rede.
4. Um canal é criado e os ativos são inseridos na grade com horário de início e término.
5. O canal é iniciado.
6. Um worker adquire o lease Redis daquele canal e atualiza seu estado.
7. Um operador solicita uma URL temporária de playback.
8. O player acessa o master, a media playlist e os segmentos através dos endpoints assinados.
9. A API monta uma janela linear com segmentos da programação atual e dos próximos itens.

## Tecnologias

| Camada | Tecnologia |
|---|---|
| Linguagem | Go 1.22+ |
| HTTP | `chi` |
| PostgreSQL | PostgreSQL 16 + `pgx` |
| Cache e locks | Redis 7 + `go-redis` |
| HLS | Parser e builder próprios |
| Autenticação | Bearer tokens, bcrypt e HMAC-SHA256 |
| Observabilidade | `slog` JSON + métricas Prometheus |
| Empacotamento | Docker Compose |
| CI | GitHub Actions |

## Pré-requisitos

Para execução completa, instale:

- Go 1.22 ou superior;
- Docker Engine e Docker Compose v2;
- `curl`, `jq` ou o parser simples incluído nos scripts;
- acesso à rede para buscar origens HLS públicas ou permitidas;
- PostgreSQL e Redis quando a execução não usar Compose.

## Execução local

Suba toda a stack de desenvolvimento:

```bash
docker compose up --build -d
```

Verifique saúde e readiness:

```bash
curl -f http://localhost:8080/healthz
curl -f http://localhost:8080/readyz
curl -f http://localhost:8080/version
curl -f http://localhost:8080/metrics
```

Execute o fluxo end-to-end:

```bash
./tests/integration/e2e.sh
./tests/integration/contract.sh
```

Para desligar e remover os volumes locais:

```bash
docker compose down -v
```

Execução direta da API, quando PostgreSQL e Redis já estiverem disponíveis:

```bash
make run
```

## Configuração

O Compose local possui valores de desenvolvimento. Em staging e produção, injete as variáveis por secret manager ou pelo ambiente do serviço. O template está em [`deployments/staging/.env.example`](deployments/staging/.env.example).

Principais variáveis:

| Variável | Finalidade |
|---|---|
| `APP_ENV` | Ambiente (`development`, `staging` ou `production`) |
| `HTTP_ADDR` | Endereço de escuta HTTP |
| `DATABASE_URL` | DSN do PostgreSQL |
| `REDIS_ADDR` | Endereço do Redis |
| `API_KEY` | Chave usada somente no bootstrap inicial |
| `AUTH_TOKEN_SECRET` | Segredo dos tokens de autenticação |
| `PLAYBACK_TOKEN_SECRET` | Segredo das URLs de playback |
| `AUTH_TOKEN_TTL_SECONDS` | TTL do token bearer |
| `PLAYBACK_TOKEN_TTL_SECONDS` | TTL da URL de playback |
| `ALLOW_PRIVATE_HLS` | Permite origens privadas; deve ser `false` em produção |
| `HLS_MAX_MANIFEST_BYTES` | Limite do tamanho do manifest |
| `HLS_REQUEST_TIMEOUT_SECONDS` | Timeout de busca do HLS de origem |
| `RATE_LIMIT_PER_MINUTE` | Limite de requisições por IP |
| `CORS_ALLOWED_ORIGIN` | Origem autorizada do player |

Em `APP_ENV=production`, a aplicação aplica validações fail-fast: segredos devem ser suficientemente longos e distintos, origens privadas devem estar desativadas e os limites/TTL precisam ser válidos.

## Autenticação e RBAC

As rotas de negócio `/v1/*` exigem:

```http
Authorization: Bearer <access_token>
```

A `API_KEY` não é uma credencial de operação. Ela serve apenas para criar o primeiro administrador através do bootstrap:

```bash
curl -X POST http://localhost:8080/v1/auth/bootstrap \
  -H 'X-API-Key: <API_KEY>' \
  -H 'Content-Type: application/json' \
  -d '{
    "email": "admin@example.com",
    "password": "uma-senha-com-pelo-menos-12-caracteres",
    "organization_name": "Minha Organização"
  }'
```

Depois, faça login:

```bash
curl -X POST http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{
    "email": "admin@example.com",
    "password": "uma-senha-com-pelo-menos-12-caracteres"
  }'
```

Papéis disponíveis:

- **Administrador (`administrador`)**: usuários, papéis, equipes e todas as operações da organização.
- **Operador (`operador`)**: inicia/para canais e emite URLs de playback.
- **Editor (`editor`)**: gerencia ativos, canais e programação.
- **Visualizador (`visualizador`)**: acesso de leitura permitido.

A alteração de papel ou remoção de associação tem efeito imediato porque a associação organizacional é consultada novamente durante as requisições protegidas.

### Matriz de autorização

As permissões de escrita são definidas em uma política declarativa por operação, aplicada pelo middleware antes do handler. O papel organizacional define a capacidade global; quando a operação informa um `tenant_id`, o handler também exige associação explícita do usuário àquele tenant. Portanto, ter um papel organizacional permitido não ignora o isolamento de tenant.

| Operação | Administrador | Editor | Operador | Visualizador |
|---|:---:|:---:|:---:|:---:|
| Gerenciar tenants, usuários e equipes | sim | não | não | não |
| Criar bibliotecas, uploads e concluir uploads | sim | sim | não | não |
| Ajustar quota | sim | não | não | não |
| Criar e validar assets | sim | sim | não | não |
| Revisar assets | sim | sim | não | não |
| Criar canais | sim | sim | não | não |
| Iniciar/parar canais e emitir playback | sim | não | sim | não |
| Criar, alterar e remover programação | sim | sim | não | não |
| Reprocessar jobs de mídia `dead` | sim | não | sim | não |

Leituras são permitidas somente quando o recurso pertence à organização e, para recursos privados, quando o usuário está associado ao tenant. Um administrador organizacional não recebe acesso automático a um tenant privado sem essa associação. Alterações de papel ou remoção de associação são reavaliadas na próxima requisição e têm efeito imediato.

## Multi-tenancy

A identidade é organizada em:

- organizações;
- usuários;
- associações usuário-organização;
- equipes;
- membros de equipe;
- papéis por organização;
- eventos de auditoria.

Canais, ativos e itens da grade são isolados por organização. As consultas HTTP filtram pelo tenant do token e as chaves estrangeiras compostas no banco reforçam que uma grade não associe recursos de organizações diferentes.

Operações com `tenant_id` exigem associação explícita do usuário em `tenant_memberships`; pertencer à organização, por si só, não autoriza acessar a biblioteca de um tenant. A migração `005_production_hardening.sql` faz o backfill inicial para administradores existentes, e o criador de um tenant recebe sua associação automaticamente.

Administradores gerenciam as associações por tenant através de `POST /v1/tenants/{tenantID}/members` e `DELETE /v1/tenants/{tenantID}/members/{userID}`. Consultas de asset, canal, programação, manifest e playback também revalidam o escopo do usuário, e não dependem somente do `organization_id` do token.

As listagens de canais e a fila de revisão aplicam o escopo de tenant diretamente no PostgreSQL com `EXISTS` em `tenant_memberships`. Isso evita carregar recursos da organização e executar uma consulta de autorização para cada item retornado.

Operações de escrita autenticadas registram ator, organização, ação, recurso, status HTTP e request ID na tabela de auditoria.

## Ingestão HLS e SSRF

Somente URLs HTTP(S) de HLS são aceitas. O backend não recebe MP4, MPEG-TS ou uploads para transcodificação.

Proteções aplicadas:

- bloqueio de `localhost`, loopback, redes privadas, link-local, multicast e endereços não especificados;
- limite de tamanho do manifest;
- timeout configurável;
- no máximo três redirecionamentos;
- validação de master ou media playlist;
- bloqueio de origens privadas por padrão.

Para fixtures locais do Compose, `ALLOW_PRIVATE_HLS=true` é usado somente no ambiente de desenvolvimento. Em staging e produção, mantenha `ALLOW_PRIVATE_HLS=false`.

### Recuperação do worker de mídia

Jobs de conversão usam lease de 15 minutos. Uma instância que cai deixa o job em `processing`, mas outra instância pode retomá-lo após a expiração do lease. Falhas transitórias voltam para `queued` até cinco tentativas; depois disso o job vai para `dead` e conserva a mensagem de erro para operação. A conclusão do job atualiza job, asset, objeto e quota na mesma transação; saídas HLS parciais são removidas quando a conversão falha.

## Playback assinado

Um administrador ou operador pode emitir uma URL temporária por canal:

```bash
curl -X POST \
  -H 'Authorization: Bearer <ACCESS_TOKEN>' \
  http://localhost:8080/v1/channels/<CHANNEL_ID>/playback
```

A resposta contém uma URL relativa e o horário de expiração. O player usa os endpoints públicos de playback sem receber credenciais administrativas.

Endpoints principais:

```text
GET /playback/{channelID}/master.m3u8
GET /playback/{channelID}/media/{variant}.m3u8
GET /playback/{channelID}/segment
```

As assinaturas HMAC-SHA256 são vinculadas ao canal e ao recurso. A API rejeita token ausente, inválido, expirado ou usado em outro canal/recurso. Tokens expirados retornam `410 Gone`.

Os manifests e segmentos usam headers de proteção, incluindo `Cache-Control: private, no-store` quando apropriado e `Referrer-Policy: no-referrer`. Os manifests possuem `ETag` SHA-256 e suportam `304 Not Modified`.

Nunca registre URLs de playback em logs, tickets ou ferramentas de analytics. Elas funcionam como credenciais temporárias.

## Manifest Builder

O builder:

- lê master e media playlists HLS;
- seleciona variantes;
- resolve URLs relativas;
- calcula a posição atual na grade;
- mantém uma janela de até 12 segmentos;
- concatena itens consecutivos da programação;
- insere descontinuidades entre ativos;
- preserva `#EXT-X-DATERANGE`;
- preserva `#EXT-X-DISCONTINUITY-SEQUENCE`;
- calcula `#EXT-X-MEDIA-SEQUENCE`;
- suporta `loop_enabled=true` para continuidade após o fim da grade;
- não finaliza o canal com `#EXT-X-ENDLIST`.

A duração do loop é calculada a partir dos horários da grade e, quando necessário, da duração conhecida dos ativos. O relógio é reposicionado no início do ciclo depois do último item.

## Alta disponibilidade

O worker de playout usa Redis para:

- lease por canal;
- renovação do lease;
- fencing token monotônico;
- escrita atômica de estado;
- prevenção contra publicação atrasada por um worker que perdeu liderança.

Múltiplos workers podem executar simultaneamente, mas apenas o processo que possui o lease e o fencing token válido deve publicar o estado do canal.

Smoke test local:

```bash
make ha-smoke
```

O teste verifica exclusão mútua da aquisição, monotonicidade do fencing token e preservação do proprietário do lease.

## Observabilidade

A API fornece:

```text
GET /healthz
GET /readyz
GET /metrics
GET /version
```

O `media-worker` expõe `GET /metrics` em `MEDIA_METRICS_ADDR` (padrão `:9091`). O endpoint registra jobs concluídos e falhos, duração do processamento por resultado e o estado atual (`starting`, `idle`, `processing`, `stopping` ou `failed`). O Compose e o Prometheus já incluem esse alvo separado da API.

Uploads que permanecem em `uploading` além de `MEDIA_ABANDONED_UPLOAD_TTL_HOURS` (padrão: 24 horas) são limpos periodicamente pelo `media-worker`. A rotina remove o objeto de quarentena, libera a quota reservada, marca o registro como `deleted` e invalida o asset pendente associado. O intervalo padrão é `MEDIA_CLEANUP_INTERVAL_SECONDS=900`; falhas de storage deixam o upload pendente para uma nova tentativa.

O logging usa JSON estruturado via `slog` e inclui método, caminho, status, duração e `X-Request-ID`.

As métricas Prometheus incluem:

- `vod2live_http_requests_total`;
- `vod2live_http_errors_total`;
- `vod2live_http_requests_by_status_total`;
- `vod2live_http_request_duration_seconds`;
- `vod2live_media_jobs_total{status="completed|failed"}`;
- `vod2live_media_job_duration_seconds`;
- `vod2live_media_worker_state{state="..."}`;
- `vod2live_auth_failures_total`;
- `vod2live_manifest_requests_total`;
- `vod2live_ready`;
- `vod2live_build_info`.

Configuração e alertas:

- [`deployments/observability/prometheus.yml`](deployments/observability/prometheus.yml)
- [`deployments/observability/rules.yml`](deployments/observability/rules.yml)

O endpoint `/metrics` deve ser restringido por rede, ingress ou autenticação do sistema de monitoramento em produção.

Jobs em estado `dead` podem ser reprocessados por administrador ou operador através de `POST /v1/media/jobs/{jobID}/reprocess`. A operação exige o mesmo escopo de organização e tenant, restaura o objeto de origem para `uploaded`, preserva `attempts` e `last_error` e acrescenta cinco tentativas ao orçamento do worker.

## Banco, backup e recuperação

O PostgreSQL é a fonte persistente do sistema. Redis contém estado derivado, cache e locks; sua perda não deve destruir a configuração ou a grade, mas exige reaquisição dos leases pelos workers.

Com a stack ativa:

```bash
make db-backup
make db-verify-backup BACKUP_FILE=backups/arquivo.dump
```

Restore é destrutivo e exige confirmação explícita:

```bash
make db-restore BACKUP_FILE=backups/arquivo.dump
```

As migrações são reaplicáveis e podem ser verificadas com:

```bash
make verify-migrations
```

Em ambientes persistentes, use `make migrate`. O executor mantém o advisory lock na mesma sessão PostgreSQL, aplica os arquivos em ordem lexicográfica, registra a versão e o SHA-256 em `schema_migrations` e interrompe a execução se um arquivo já aplicado tiver sido alterado. Para staging, injete `DATABASE_URL` pelo secret manager e execute `docker compose --env-file deployments/staging/.env -f deployments/staging/docker-compose.staging.yml --profile tools run --rm migration-runner` antes de promover os serviços; o CI também aplica as migrações antes de iniciar API e playout.

O runbook de recuperação, RPO/RTO e rollback está em [`deployments/staging/RUNBOOK.md`](deployments/staging/RUNBOOK.md).

Objetivos operacionais documentados:

- **RPO alvo:** 15 minutos;
- **RTO alvo:** 30 minutos.

Esses objetivos dependem de backups externos, armazenamento durável, secret manager, automação de provisionamento e testes periódicos de restauração.

## Biblioteca, quotas e conversão HLS

O pipeline de mídia permite que a organização mantenha uma biblioteca compartilhada e que cada tenant tenha uma biblioteca privada com quota independente. Conteúdo de biblioteca da organização pode ser usado por canais de qualquer tenant da mesma organização; conteúdo de biblioteca privada só pode ser associado a canais do tenant proprietário. Canais recebem `tenant_id` opcional: canal sem tenant só pode usar conteúdo compartilhado da organização. O PostgreSQL controla metadados, reservas, escopo e consumo; o MinIO guarda objetos; o worker executa a conversão em segundo plano.

```text
upload → quarentena → conversão FFmpeg → aguardando revisão → aprovado/rejeitado → grade
```

Endpoints adicionados:

```text
GET  /v1/libraries
POST /v1/libraries
GET  /v1/storage/quota
GET    /v1/media/jobs?tenant_id={tenantID}
POST   /v1/media/jobs/{jobID}/reprocess
GET    /v1/uploads?library_id={libraryID}
POST /v1/libraries/{libraryID}/uploads
POST /v1/uploads/{uploadID}/complete
GET  /v1/review/assets
POST /v1/assets/{id}/review
GET  /v1/assets/{id}/review/history
```

`GET /v1/channels` e `GET /v1/review/assets` aceitam `limit` e `offset` (por exemplo, `?limit=50&offset=0`). O padrão é 50 itens por página, o máximo é 200 e valores inválidos retornam HTTP 400. A resposta continua sendo um array; o filtro de tenant permanece aplicado no SQL antes da paginação.

O upload reserva quota antes de gerar a URL pré-assinada. Ao ser confirmado, cria um job assíncrono. O `media-worker` baixa o original do MinIO, executa FFmpeg em container, produz um master HLS com variante 720p e envia playlists/segmentos de volta ao MinIO. A conversão bem-sucedida marca o asset como tecnicamente `valid`, mas seu estado editorial permanece `pending`; ele não pode ser agendado até que um administrador/editor o aprove pela fila de revisão. A decisão e o revisor ficam registrados; rejeições desabilitam itens de grade existentes. A verificação é feita no backend ao criar/reativar programação e ao consultar a grade usada pelo manifest.

Assets de bibliotecas privadas carregam o `tenant_id` da biblioteca. A API e as consultas de programação permitem conteúdo privado somente quando o `tenant_id` do canal coincide; bibliotecas compartilhadas da organização têm `tenant_id` nulo e podem ser usadas por qualquer tenant daquela organização. A API exige associação do usuário ao tenant para criar bibliotecas, canais, uploads e consultar quota/conteúdo privado. Uploads são limitados a 20 GiB e tipos de mídia suportados; o tamanho real é conferido no storage contra a reserva antes do job ser criado.

O Compose local inclui `minio` e `media-worker`. Como a imagem comunitária precompilada do MinIO deixou de estar disponível nos registries públicos, o container local compila a release upstream fixada pelo commit em `deployments/minio/Dockerfile`; esse build serve apenas para desenvolvimento e CI, não como imagem de produção. O override de staging remove as dependências locais e usa PostgreSQL, Redis e MinIO externos. Para produção, use bucket privado, credenciais via secret manager, limites de CPU/memória no worker e políticas de retenção para originais, temporários e HLS publicado.

`GET /v1/media/jobs` retorna contagens de jobs `queued`, `processing`, `completed`, `failed` e `dead`, permitindo alertas e acompanhamento por organização ou tenant.

## Staging e CDN

O Compose de staging usa imagens imutáveis e secrets injetados pelo ambiente:

```bash
docker compose \
  -f docker-compose.yml \
  -f deployments/staging/docker-compose.staging.yml \
  config
```

O arquivo [`deployments/staging/.env.example`](deployments/staging/.env.example) contém somente nomes e exemplos, nunca segredos reais.

A estratégia CDN recomendada está em [`deployments/staging/CDN.md`](deployments/staging/CDN.md). Em resumo:

- master e media playlists devem ter TTL curto;
- segmentos devem ser cacheados por URL completa;
- a CDN deve respeitar ETag e `304`;
- API administrativa não deve ficar atrás de cache público;
- query strings de assinatura precisam permanecer na chave de cache ou ser validadas no edge;
- CORS deve apontar para o domínio real do player;
- origin, erros, hit ratio e latência devem ser monitorados.

## Testes e CI

Testes unitários:

```bash
make test
```

Formatação:

```bash
make fmt
```

Testes de integração:

```bash
make e2e
make contract
```

Carga básica de endpoints operacionais:

```bash
REQUESTS=500 CONCURRENCY=16 make load-test
```

Carga de playback. Primeiro obtenha uma URL assinada e informe-a sem expô-la em logs:

```bash
PLAYBACK_URL='http://localhost:8080/playback/...' \
REQUESTS=500 CONCURRENCY=16 \
make playback-load-test
```

O CI executa:

1. `go test ./...`;
2. validação do Compose;
3. build da API e do worker;
4. PostgreSQL, Redis e fixture HLS;
5. E2E e contrato HTTP;
6. smoke de fencing Redis;
7. carga de playback;
8. carga de endpoints operacionais;
9. verificação de migrações;
10. dump e validação básica do PostgreSQL.

A validação integrada precisa ser executada em um runner com Docker habilitado.

## Estrutura do projeto

```text
cmd/
  api/                  API HTTP principal
  playout/              Worker contínuo de playout
  scheduler/            Ponto de extensão para scheduler
  validator/            Ponto de extensão para validação
  media-worker/         Conversão assíncrona de uploads para HLS
internal/
  asset/                Ativos HLS e persistência
  auth/                 Tokens, papéis e autorização
  channel/              Canais e estado
  config/               Configuração e validação por ambiente
  db/                   Conexão e retry de dependências
  health/               Health/readiness checks
  identity/             Usuários, organizações, equipes e auditoria
  manifest/             Montagem dos manifests lineares
  metrics/              Métricas Prometheus
  observability/        Logging e middleware operacional
  playback/             URLs assinadas e proxy HLS
  playout/              Lease, fencing e estado do worker
  library/              Tenants, bibliotecas, quotas e jobs
  media/                Worker FFmpeg
  storage/              Cliente S3/MinIO
  ratelimit/             Limitação por IP
  retry/                Backoff de dependências
pkg/hls/                 Parser e renderer HLS
migrations/              Migrações PostgreSQL
scripts/                 Backup, restore, carga e validações
tests/
  fixtures/             Origem HLS local para testes
  integration/           E2E, contrato e smoke tests
deployments/
  docker/               Dockerfiles
  observability/        Prometheus e alertas
  staging/              Compose, env template e runbooks
```

## API

A especificação OpenAPI está em [`openapi.yaml`](openapi.yaml).

Endpoints administrativos principais:

```text
POST   /v1/auth/bootstrap
POST   /v1/auth/login
GET    /v1/tenants
POST   /v1/tenants
GET    /v1/libraries
POST   /v1/libraries
GET    /v1/storage/quota
POST   /v1/storage/quota
POST   /v1/libraries/{libraryID}/uploads
POST   /v1/uploads/{uploadID}/complete
GET    /v1/users
POST   /v1/users
PATCH  /v1/users/{userID}/role
DELETE /v1/users/{userID}
POST   /v1/teams
POST   /v1/teams/{teamID}/members
DELETE /v1/teams/{teamID}/members/{userID}
POST   /v1/assets
GET    /v1/assets/{id}
POST   /v1/assets/{id}/validate
POST   /v1/channels
GET    /v1/channels
GET    /v1/channels/{id}
POST   /v1/channels/{id}/start
POST   /v1/channels/{id}/stop
GET    /v1/channels/{id}/status
POST   /v1/channels/{id}/schedule
GET    /v1/channels/{id}/schedule
PATCH  /v1/schedule/{id}
DELETE /v1/schedule/{id}
POST   /v1/channels/{id}/playback
```

Endpoints operacionais públicos:

```text
GET /healthz
GET /readyz
GET /metrics
GET /version
```

## Limitações e próximos passos

Antes de produção, ainda é necessário concluir e evidenciar no ambiente-alvo:

- teste de concorrência com múltiplos workers reais e interrupção de processos;
- teste de carga com volume, duração e distribuição semelhantes ao tráfego esperado;
- validação de failover PostgreSQL/Redis e comportamento durante perda de rede;
- provisionamento real de staging com secret manager;
- integração com o provedor CDN escolhido;
- teste de restauração completo medindo RPO e RTO;
- política de retenção e cópia externa dos backups;
- dashboards operacionais e alertas integrados ao canal de incidentes;
- validação do comportamento de sliding window com diferentes durações e eventos SCTE-35;
- revisão de segurança independente antes da exposição pública.

O pipeline atual implementa ingestão de arquivo e conversão assíncrona para HLS em uma variante inicial 720p. Ainda não implementa perfis múltiplos configuráveis por organização, DRM, SCTE-35 completo, gravação de live input, limpeza automática de objetos abandonados, reconciliação periódica de quota ou controle visual de grade. Esses recursos são os próximos incrementos naturais.

## Licença e contribuição

Consulte as políticas do repositório antes de distribuir ou modificar o software. Pull requests devem incluir testes, atualização da documentação quando necessário e evidência dos gates aplicáveis.
