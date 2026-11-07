# VOD2Live Backend

Backend modular para montar canais lineares usando somente fontes HLS (`master.m3u8` ou media playlist). O projeto não faz transcodificação: valida manifests, persiste a grade, mantém o estado do playout e publica manifests HLS lineares.

## Componentes

- `api`: REST para canais, assets e grade.
- `validator`: leitura e validação de URLs HLS.
- `scheduler`: resolução da programação publicada.
- `playout`: relógio e estado por canal.
- `manifest`: montagem da janela HLS de saída.
- PostgreSQL: configuração, catálogo e grade.
- Redis: locks, cache e estado quente.

## Executar

```bash
docker compose up --build
curl http://localhost:8080/healthz
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
- `POST /v1/assets`
- `GET /v1/assets/{id}`
- `POST /v1/assets/{id}/validate`
- `POST /v1/channels`
- `GET /v1/channels`
- `GET /v1/channels/{id}`
- `POST /v1/channels/{id}/schedule`
- `GET /v1/channels/{id}/schedule`
- `GET /v1/channels/{id}/master.m3u8`

## Limites atuais do esqueleto

A montagem real de manifests e a persistência dos repositórios estão isoladas por interfaces para implementação incremental. Não são aceitos MP4, MPEG-TS ou upload de arquivos.
