# Estratégia CDN para HLS

A CDN deve ficar na frente dos endpoints `/playback/*`, mantendo a API administrativa fora do cache público. O origin deve ser acessível somente pela CDN ou por uma rede privada.

## Regras recomendadas

- **Master e media playlists:** respeitar `ETag`; usar `If-None-Match`; TTL curto (1–3 segundos para media, 10–30 segundos para master) e `stale-if-error` controlado.
- **Segmentos:** cache por URL completa com TTL igual ou maior que a duração do segmento; manter query string de assinatura na chave de cache ou validar a assinatura no edge antes do cache.
- **Segurança:** encaminhar apenas query strings necessárias, remover `Authorization` do cache key, bloquear métodos diferentes de `GET`/`HEAD` e restringir origem por allowlist.
- **CORS:** configurar `CORS_ALLOWED_ORIGIN` para o domínio real do player, nunca `*` em produção.
- **Purge:** não fazer purge por requisição; publicar novos manifests por ETag e deixar a expiração natural dos segmentos.
- **Observabilidade:** coletar hit ratio, 4xx/5xx, latência de origin, bytes e requests por canal; alertar aumento de miss ratio e erro de origin.

## Validação de staging

1. Emitir uma URL de playback com expiração curta.
2. Repetir `GET` do master e validar `200`, `ETag` e `304`.
3. Buscar uma media playlist e um segmento por duas regiões/PoPs.
4. Confirmar que URL expirada retorna `410` e que alterar `channel_id`, recurso ou assinatura retorna `403`.
5. Confirmar que o CDN não armazena headers ou tokens administrativos.
