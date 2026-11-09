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
