# Avaliacao de ai-memory

## Identificacao

| Campo | Valor |
|---|---|
| Projeto | ai-memory |
| Repositorio | `https://github.com/akitaonrails/ai-memory.git` |
| Clone local | `~/workspace/ai-memory` |
| Revisao | `3bd35154c845d5c2ab510e2c2ecae3c1d2059b19` |
| Branch | `main`, alinhada a `origin/main` |
| Estado inicial | arvore limpa |
| Data | 2026-09-04 (triagem e etapas 3/4); 2026-09-05 (Etapa 6) |
| Avaliacao | triagem estatica e etapas 3, 4 e 6 comportamentais; privacidade sob trace e suite completa nao executadas |
| Avaliador | OpenCode |
| Run IDs | `20260904T212935Z-ai-memory-stage3`; `20260904T233117Z-ai-memory-stage4`; `stage6-ai-memory-run` |
| Commits iniciais da memoria | `29645b91101fbf98d524c8766d83cd4ae1ca3367` (etapa 3); `22d740759c5d8bf0218e3452a3562f1477845978` (etapa 4) |

## Hipotese

ai-memory e o comparador end-to-end mais proximo, mas daemon, spool anterior a sanitizacao e persistencia de sessoes podem elimina-lo como base, preservando conceitos de wiki, supersessao e proposta.

## Escopo e metodo

Foram lidos manifestos, migracoes, CLI, hooks, store, wiki, consolidacao, MCP e testes. Na etapa 3, o binario foi compilado e o daemon foi executado com o corpus comum para avaliar FTS, edicao externa e rebuild. Na etapa 4, o fluxo de auto-improvement foi exercitado com captura sintetica, provider deterministico em loopback, aprovacao manual e testes focados. Na etapa 6, hooks reais e o bridge MCP foram carregados por Claude Code em configuracao descartavel, sem credenciais. Privacidade sob trace e a suite completa permaneceram excluidas.

```bash
git rev-parse HEAD
git branch --show-current
git remote get-url origin
git status --short --branch
git log -1 --format='%cI %s'
git rev-list --count HEAD
git rev-list --count --since='2026-08-05T00:00:00Z' HEAD
git shortlog -sne HEAD
```

Os comandos de ponto fixo retornaram `0`; a duracao nao foi medida. O projeto e um workspace Rust 2024 com toolchain 1.95, Tokio, Axum/MCP, SQLite/FTS5, libgit2, watcher e clientes LLM (`Cargo.toml:1-37,97-138`; `rust-toolchain.toml:1-4`). O historico local tem 1.480 commits, 309 desde 2026-08-05 e multiplos contribuidores; a revisao e de 2026-09-01.

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). Os runs comportamentais usaram Bubblewrap 0.6.1, Rust/Cargo 1.95.0 e ai-memory 1.39.0. Dependencias foram provisionadas com rede e o trust bundle do sistema montado somente para leitura; os builds de 405,44 s (etapa 3) e 395,64 s (etapa 4), os testes e todos os fluxos avaliados ocorreram sem rede externa e sem variaveis de proxy.

## Fluxo estatico

| Estagio | Evidencia no clone | Conclusao |
|---|---|---|
| Entrada | `crates/ai-memory-hooks/src/router.rs:576-671,756-839`; `crates/ai-memory-hooks/src/payload.rs:404-547` | Hooks HTTP extraem sessao, cwd, prompt e resultados de ferramentas. |
| Spool | `crates/ai-memory-cli/src/commands/hook.rs:483-648`; `crates/ai-memory-cli/src/commands/hook_spool.rs:205-275,487-505` | O hook nativo persiste o envelope antes de envia-lo ao servidor. |
| Sanitizacao | `crates/ai-memory-hooks/src/router.rs:2479-2508`; `crates/ai-memory-store/src/writer.rs:811-875` | A fronteira tipada protege o writer SQLite, mas ocorre depois do spool. |
| Persistencia | `crates/ai-memory-store/src/ops.rs:1310-1432`; `crates/ai-memory-hooks/src/router.rs:2616-2625,2670-2865` | Observacao, log, pagina de sessao, handoff e commit sao persistidos em sequencia. |
| Consolidacao | `crates/ai-memory-consolidate/src/consolidator.rs:130-237,418-559` | Consolidacao LLM comum sobrescreve paginas diretamente; auto-improve possui outro fluxo com proposta. |
| Proposta | `crates/ai-memory-consolidate/src/auto_improve.rs:427-549,1542-1609`; `crates/ai-memory-store/src/auto_improve.rs:570-828,922-1059` | Reviewer gera proposta com snapshot/hash e detecta drift. |
| Recuperacao | `crates/ai-memory-mcp/src/server.rs:1813-2098,3057-3123` | Busca combina paginas e observacoes; leitura completa prefere Markdown. |
| Reindexacao | `crates/ai-memory-cli/src/commands/reindex.rs:29-82`; `crates/ai-memory-wiki/src/wiki.rs:1354-1431` | Reconstroi paginas, links, FTS e entidades, nao sessoes e auditoria; a etapa 3 confirmou a pagina e a FTS com o corpus comum. |

## Armazenamento e dependencias

A wiki Markdown fica em um repositorio Git proprio dentro do data dir (`crates/ai-memory-wiki/src/wiki.rs:80-135`). Paginas sao canonicas, mas SQLite tambem guarda sessoes, observacoes, handoffs, propostas e auditoria que nao podem ser reconstruidos da wiki. As propostas sao explicitamente DB-authoritative (`crates/ai-memory-store/migrations/V21__auto_improve_pending_proposals.sql:1-45`). `commit_all` adiciona toda a arvore da wiki (`crates/ai-memory-wiki/src/git.rs:69-129`), o que exigira isolamento antes de qualquer reutilizacao.

O fluxo de hooks e MCP HTTP requer `ai-memory serve`, que abre banco, watcher e schedulers (`crates/ai-memory-cli/src/commands/serve.rs:619-807,920-1205`). SQLite e Git sao embutidos, e operacao sem cloud e possivel em principio. Consolidacao e review exigem um LLM local ou remoto; MCP stdio sozinho nao recebe hooks.

## Etapa 3: armazenamento e recuperacao

O run bruto esta em `/tmp/git-push-memories-phase-1/20260904T212935Z-ai-memory-stage3-evidence`. A copia executavel permaneceu destacada na revisao fixa e terminou limpa; o repositorio de codigo sintetico tambem terminou limpo. Como a CLI fixa a wiki em `<data-dir>/wiki`, esse diretorio foi o repositorio de memoria e permaneceu separado de `/workspace/code`. Providers LLM/embedding, maintenance e auto-improve foram desabilitados.

Configuracao efetiva relevante; o `token_pepper` sintetico gerado por `init` foi omitido:

```text
DATA=/home/phase-1/ai-memory-data
REBUILD=/home/phase-1/ai-memory-rebuild
WORKSPACE=phase1
PROJECT=checkoutapi
PAGE=services/checkout-api.md
AI_MEMORY_MAINTENANCE__ENABLED=false
AI_MEMORY_AUTO_IMPROVE__SCHEDULER__ENABLED=false
AI_MEMORY_BACKUP_DIR=/home/phase-1/migration-backups
AI_MEMORY_LLM_PROVIDER=ausente
AI_MEMORY_EMBEDDING_PROVIDER=ausente
```

O subfluxo principal da etapa 3, sempre por `run_isolated`, foi:

```bash
ai-memory --data-dir "$DATA" init
ai-memory --data-dir "$DATA" serve --transport http --bind 127.0.0.1:49374 --workspace phase1 --project checkoutapi &
SERVER_PID=$!
for _ in $(seq 1 1200); do
  curl -fsS http://127.0.0.1:49374/admin/status >/dev/null && break
  kill -0 "$SERVER_PID"
  sleep 0.1
done
PROJECT_META=
for meta in "$DATA"/wiki/*/*/_meta.md; do
  grep -Fqx 'project: checkoutapi' "$meta" || continue
  workspace_meta="$(dirname "$(dirname "$meta")")/_meta.md"
  grep -Fqx 'workspace: phase1' "$workspace_meta" || continue
  PROJECT_META="$meta"
  break
done
test -n "$PROJECT_META"
PROJECT_ROOT="${PROJECT_META%/_meta.md}"
mkdir -p "$PROJECT_ROOT/services"
cp /fixtures/memory-initial/services/checkout-api.md "$PROJECT_ROOT/services/checkout-api.md"
ai-memory --data-dir "$DATA" commit -m "seed initial memory"
ai-memory --data-dir "$DATA" search "Qual e o timeout de checkout-api e qual time e responsavel?" --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" read-page --path services/checkout-api.md --workspace phase1 --project checkoutapi --json
cp /fixtures/memory-expected/services/checkout-api.md "$PROJECT_ROOT/services/checkout-api.md"
ai-memory --data-dir "$DATA" search "Qual e o timeout de checkout-api e qual time e responsavel?" --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" read-page --path services/checkout-api.md --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" search 'team-a OR "cinco segundos"' --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" search 'team-b OR "tres segundos"' --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" status --json
# com o servidor parado:
kill -INT "$SERVER_PID"
wait "$SERVER_PID"
cp -a "$DATA" "$REBUILD"
rm -f "$REBUILD/db/memory.sqlite" "$REBUILD/db/memory.sqlite-wal" "$REBUILD/db/memory.sqlite-shm"
ai-memory --data-dir "$REBUILD" reindex
ai-memory --data-dir "$REBUILD" serve --transport http --bind 127.0.0.1:49374 --workspace phase1 --project checkoutapi --no-watcher &
SERVER_PID=$!
for _ in $(seq 1 1200); do
  curl -fsS http://127.0.0.1:49374/admin/status >/dev/null && break
  kill -0 "$SERVER_PID"
  sleep 0.1
done
ai-memory --data-dir "$REBUILD" search "Qual e o timeout de checkout-api e qual time e responsavel?" --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$REBUILD" read-page --path services/checkout-api.md --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$REBUILD" search 'team-a OR "cinco segundos"' --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$REBUILD" search 'team-b OR "tres segundos"' --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$REBUILD" status --json
kill -INT "$SERVER_PID"
wait "$SERVER_PID"
```

O helper `run_isolated` e a preparacao dos repositorios seguiram [`evaluation-protocol.md`](../evaluation-protocol.md#preparacao-dos-repositorios), com a adaptacao documentada de que a wiki Git e criada pela propria ferramenta. A inicializacao foi aguardada pelo polling mostrado acima. Os JSON completos e os arquivos de evidencia nomeados abaixo estao no `EVIDENCE_ROOT` indicado.

| Rotulo de evidencia | Codigo | Duracao | stdout | stderr |
|---|---:|---:|---|---|
| `init` | 0 | 0,00 s | vazio | log de layout/config criados |
| `server-initial` | 0 | ciclo completo | vazio | servidor pronto e encerrado por `SIGINT` |
| `commit-initial` | 0 | 0,01 s | commit `29645b9` | log informativo |
| `query-initial` | 0 | 0,01 s | 1 resultado; 27 palavras/345 bytes | log informativo |
| `read-initial` | 0 | 0,01 s | cinco segundos/`team-a` | log informativo |
| `query-after-edit` | 0 | 0,01 s | 1 resultado; 27 palavras/344 bytes | log informativo |
| `read-after-edit` | 0 | 0,01 s | tres segundos/`team-b` | log informativo |
| `old-state-after-edit` | 0 | 0,01 s | `[]` | log informativo |
| `new-state-after-edit` | 0 | 0,01 s | 1 resultado | log informativo |
| `status-after-edit` | 0 | 0,01 s | 2 paginas/2 FTS | log informativo |
| `rebuild-from-markdown` | 0 | 0,12 s | 1 pagina/1 projeto/1 workspace | migracoes SQL informativas |
| `server-rebuilt` | 0 | ciclo completo | vazio | migracao OKF, servidor pronto e encerrado por `SIGINT` |
| `query-after-rebuild` | 0 | 0,02 s | 1 resultado; 27 palavras/344 bytes | log informativo |
| `read-after-rebuild` | 0 | 0,01 s | tres segundos/`team-b` | log informativo |
| `old-state-after-rebuild` | 0 | 0,01 s | `[]` | log informativo |
| `new-state-after-rebuild` | 0 | 0,01 s | 1 resultado | log informativo |
| `status-after-rebuild` | 0 | 0,01 s | 1 pagina/1 FTS, providers desabilitados | log informativo |

`flow.exit-code` e os dois `server-*.exit-code` registram `0`. `retrieval-comparison.json` confirma snippet normalizado e corpo integral iguais antes/depois do rebuild e ausencia do estado antigo. `before-removal-db.json`, `after-reindex-db.json` e `after-rebuild-server-db.json` preservam as contagens de todas as tabelas.

O `reindex` em si nao alterou o Markdown: o hash permaneceu `a25761aae03007249bd8d85a2764c2c54421d114`, e o diff ainda era somente a troca do estado inicial pelo esperado. `raw/` e `models/` estavam vazios, e nenhum provider estava configurado. O conhecimento reconstruido veio dos arquivos Markdown da wiki, incluindo seus `_meta.md`; `config.toml` forneceu apenas configuracao operacional. O banco novo nao recuperou versoes antigas, sessoes, observacoes ou handoffs. Alem da pagina atual e FTS, gerou schema, workspace, projeto, uma linha de auditoria e estado local de autenticacao.

O primeiro `serve` sobre o banco reconstruido revelou estado operacional nao reconstruivel. Como `wiki_migrations` voltou vazio, a migracao OKF foi reaplicada: criou os commits `pre-okf-migration checkpoint` e `okf-migration: conform wiki to OKF v0.2`, adicionou frontmatter a pagina, criou `index.md`, gerou `pre-migration-backup.json` e um backup de 257.949 bytes fora do data dir. O hash final da pagina passou para `ea45a7d6d85b551e8a5ae298a17f75faeffb90f6`; o diff final contra o commit inicial contem a pagina atualizada com frontmatter e o novo `index.md`. O backfill posterior ainda deixou os dois `_meta.md` modificados. A recuperacao permaneceu correta, mas remover o SQLite pode causar mutacao e commit automaticos do corpus na proxima inicializacao.

O inventario final registrou `config.toml` de 13.464 bytes em cada data dir, logs de 150.917 e 300.046 bytes, bancos com WAL de aproximadamente 2,5 MB e SHM de 32.768 bytes, alem do receipt e backup da migracao. Esses itens sao persistencia operacional adicional, nao fontes usadas para reindexar o conhecimento.

As latencias de consulta medem apenas o cliente contra um daemon ja residente; o RSS do servidor nao foi isolado. Elas demonstram operacao offline, mas nao podem ser comparadas diretamente com os processos frios do Basic Memory.

## Etapa 4: consolidacao do estado atual

O run bruto esta em `/tmp/git-push-memories-phase-1/20260904T233117Z-ai-memory-stage4-evidence`; os scripts completos de preparacao, mock, fluxo e testes estao no subdiretorio `harness/`. Para repetir, copie esses quatro arquivos para `/tmp/opencode/` e execute `bash /tmp/opencode/run-ai-memory-stage4.sh`. A copia executavel permaneceu destacada na revisao fixa e limpa. O fluxo inteiro executou em um namespace de rede sem acesso externo; apenas o daemon do produto e um provider OpenAI-compatible deterministico em `127.0.0.1:49375` compartilharam o loopback. O provider nao avalia qualidade de modelo: ele torna reproduzivel o contrato de proposta e devolve uma redacao ambigua antes da rejeicao e uma correcao depois dela. Para testar idempotencia do executor, ele volta a emitir a mesma operacao quando o alvo ja esta atualizado.

A pagina comum foi adaptada apenas de `services/checkout-api.md` para `procedures/checkout-api.md`, pois a revisao so oferece patch de paginas existentes em `procedures/` e `_rules/`. O corpo inicial permaneceu igual ao fixture. Configuracao efetiva relevante:

```text
AI_MEMORY_LLM_PROVIDER=openai-compat
AI_MEMORY_LLM_MODEL=phase1-mock
AI_MEMORY_LLM_BASE_URL=http://127.0.0.1:49375/v1
AI_MEMORY_LLM_COMPAT_STRICT=true
AI_MEMORY_CONSOLIDATE_ON_SESSION_END=false
AI_MEMORY_AUTO_IMPROVE__REQUIRE_APPROVAL=true
AI_MEMORY_AUTO_IMPROVE__SCHEDULER__ENABLED=false
AI_MEMORY_MAINTENANCE__ENABLED=false
```

O fluxo principal foi:

```bash
ai-memory --data-dir "$DATA" write-page \
  --workspace phase1 --project checkoutapi \
  --path procedures/checkout-api.md --kind procedure --tier procedural \
  --body "$(< /fixtures/memory-initial/services/checkout-api.md)"
curl -fsS -H 'content-type: application/json' \
  --data-binary @hook-batch-input.json http://127.0.0.1:49374/hook/batch
ai-memory --data-dir "$DATA" auto-improve \
  --workspace phase1 --project checkoutapi --session-id "$SESSION_ID" \
  --min-observations 1 --min-session-duration-secs 0 --max-proposals 1 --json
ai-memory --data-dir "$DATA" pending-writes show "$P1" --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" pending-writes diff "$P1" --workspace phase1 --project checkoutapi --json
# repetir enquanto P1 esta pendente testa deduplicacao por alvo
ai-memory --data-dir "$DATA" auto-improve \
  --workspace phase1 --project checkoutapi --session-id "$SESSION_ID" \
  --min-observations 1 --min-session-duration-secs 0 --max-proposals 1 --json
ai-memory --data-dir "$DATA" pending-writes reject "$P1" \
  --workspace phase1 --project checkoutapi \
  --reason 'remova a referencia ao estado antigo e use afirmacoes diretas' --json
ai-memory --data-dir "$DATA" auto-improve \
  --workspace phase1 --project checkoutapi --session-id "$SESSION_ID" \
  --min-observations 1 --min-session-duration-secs 0 --max-proposals 1 --json
ai-memory --data-dir "$DATA" pending-writes show "$P2" --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" pending-writes diff "$P2" --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" pending-writes approve "$P2" --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" read-page \
  --path procedures/checkout-api.md --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" search \
  'team-a OR "cinco segundos"' --workspace phase1 --project checkoutapi --json
# repetir a mesma operacao e aprova-la, para idempotencia
ai-memory --data-dir "$DATA" auto-improve \
  --workspace phase1 --project checkoutapi --session-id "$SESSION_ID" \
  --min-observations 1 --min-session-duration-secs 0 --max-proposals 1 --json
ai-memory --data-dir "$DATA" pending-writes diff "$P3" --workspace phase1 --project checkoutapi --json
ai-memory --data-dir "$DATA" pending-writes approve "$P3" --workspace phase1 --project checkoutapi --json
```

`hook-batch-input.json` continha apenas `session-start`, a descoberta sintetica da etapa e `session-end`, com um UUID fixo. O mock validou `response_format=json_schema`, copiou o hash de `## Operacao` fornecido pelo proprio reviewer e anexou como evidencia a pagina sintetica da sessao. Os quatro requests sanitizados estao em `mock-requests.jsonl`.

| Rotulo de evidencia | Codigo | Duracao | stdout | stderr |
|---|---:|---:|---|---|
| `seed-page` | 0 | 0,02 s | 6 palavras/81 bytes | log informativo |
| `read-initial` / `query-initial` | 0 | 0,00 s cada | 481/344 bytes | log informativo |
| `submit-discovery` | 0 | 0,02 s | 14 bytes | vazio |
| `proposal-first` | 0 | 0,01 s | 1 proposta pendente; 628 bytes | log informativo |
| `pending-list-first` / `proposal-first-show` / `proposal-first-diff` | 0 | 0,01/0,01/0,00 s | 254/769/369 bytes | log informativo |
| `read-after-first-proposal` | 0 | 0,00 s | estado inicial; 481 bytes | log informativo |
| `proposal-duplicate-while-pending` | 0 | 0,00 s | proposta ignorada pelo alvo pendente; 452 bytes | log informativo |
| `reject-first` | 0 | 0,00 s | status rejeitado; 27 bytes | log informativo |
| `proposal-first-after-reject` | 0 | 0,00 s | evento e motivo; 1.078 bytes | log informativo |
| `proposal-second` | 0 | 0,01 s | nova proposta pendente; 628 bytes | log informativo |
| `proposal-second-show` / `proposal-second-diff` | 0 | 0,01/0,00 s | 751/351 bytes | log informativo |
| `approve-second` | 0 | 0,01 s | status aprovado; 80 bytes | log informativo |
| `read-after-approve` | 0 | 0,01 s | tres segundos/`team-b`; 469 bytes | log informativo |
| `query-after-approve` / estado novo | 0 | 0,00/0,01 s | 847/780 bytes | log informativo |
| `old-state-after-approve` | 0 | 0,01 s | `[]` | log informativo |
| `proposal-repeat` / show / diff / approve | 0 | 0,01/0,00/0,00/0,01 s | 628/751/352/80 bytes | log informativo |
| `read-after-repeat` | 0 | 0,00 s | estado novo inalterado semanticamente; 469 bytes | log informativo |
| `focused-tests` | 0 | 230,06 s | 9 testes passaram | compilacao e progresso Cargo |

Todos os comandos do fluxo, o servidor e o script agregado retornaram `0`; o mock terminou com `143` pelo `SIGTERM` esperado. `stage4-assertions.json` registra treze verificacoes, incluindo a falha de estabilidade de bytes. A primeira proposta continha a referencia antiga `team-a`; `reject` e a tentativa duplicada enquanto pendente mantiveram o hash canonico `4ff2caae068d5b1541b05533a0827a8b2d4f086e`. A segunda proposta apresentou exatamente tres segundos e `team-b`; somente `approve` mudou o hash para `10f6d3304796d89d2e243f424888b470a5ecad29`.

O inventario SQLite registrou quatro runs, tres propostas (`rejected`, `approved`, `approved`), seis eventos e uma rejeicao. A tentativa enquanto havia proposta pendente foi descartada com `a proposal is already pending review for this path`. Depois do primeiro aceite, a pagina inicial ficou `is_latest=0`, a pagina nova ficou atual e a busca pelo estado antigo retornou `[]`.

A repeticao emitiu e aprovou exatamente o mesmo corpo esperado. O conteudo semantico permaneceu correto, mas o produto criou uma terceira versao, atualizou IDs de proposta/run no frontmatter e mudou o hash para `34bdbe32fee4f692dc1fcb5580175fdf837c4f7b`. Logo, a aplicacao de proposta equivalente nao e idempotente em bytes nem em versoes, embora nao reintroduza contradicao no corpo. O teste unitario de rewrite identico passa no store generico, mas nao cobre o frontmatter adicionado pelo caminho de auto-improvement.

A unicidade parcial e garantida por alvo enquanto existe uma proposta pendente, mas nao depois do aceite. Nao foi criado um objeto tipado de contradicao: a primeira redacao ainda mencionava `team-a` e foi rejeitada pela pessoa, enquanto `rejected_candidates_count` permaneceu zero. A proveniencia das tres propostas aponta para a pagina sintetica da sessao e sua quote; a rationale menciona `service.yaml`, mas o arquivo de codigo nao esta ligado diretamente a proposta. Portanto duplicidade, contradicao e evidencia foram inspecionadas, mas as duas ultimas continuam dependentes do reviewer e de revisao humana.

A proposta existe simultaneamente como estado autoritativo no SQLite e sidecar Markdown nao indexado. A rejeicao nao atualiza o `status: pending` renderizado no sidecar antigo; `pending-writes show` e o banco registram o estado real. Nao ha comando para ajustar uma proposta pendente: o fluxo suportado e rejeitar, regenerar, inspecionar e aprovar. A aprovacao alterou a pagina e adicionou IDs de proposta/run ao frontmatter, mas nao fez commit; os tres sidecars ficaram nao rastreados e a pagina modificada. A sessao e o log mensal haviam sido commitados antes, reforcando que proposta, aplicacao canonica e publicacao Git sao fronteiras distintas.

Os nove testes focados cobriram reviewer sem escrita, contexto de rejeicao, patches, aprovacao manual, show/diff/reject, supersessao, rollback por conflito, hash de baseline e rewrite identico. Todos passaram offline; a suite completa nao foi executada.

## Etapa 6: integracao com Claude Code

A evidencia complementar esta em `/tmp/stage6-ai-memory-run/evidence-stage6-rerun`. O run usou Claude Code 2.1.261, ai-memory 1.39.0, `HOME` e `CLAUDE_CONFIG_DIR` descartaveis, ambiente criado por `env -i`, IDs de sessao sinteticos e nenhum token ou credencial. O daemon ficou em loopback, com maintenance e scheduler de auto-improvement desabilitados. Esta passagem nao usou Bubblewrap, reutilizou um banco acumulado e nao registrou duracao por comando; por isso somente sustenta classificacao `parcial` sob o protocolo. `apiKeySource=none` fez cada chamada terminar imediatamente com `authentication_failed`. Ela demonstra os contratos dos hooks, MCP, handoff e falha do daemon sem expor uma sessao real.

As duas sessoes principais foram iniciadas no mesmo repositorio descartavel com `--no-session-persistence`, `--setting-sources user`, configuracoes explicitas de hooks/MCP e `--include-hook-events`:

```bash
claude -p --session-id 11111111-1111-4111-8111-111111111111 \
  --no-session-persistence --setting-sources user \
  --settings /tmp/stage6-ai-memory-home/.claude/settings.json \
  --strict-mcp-config --mcp-config /tmp/stage6-ai-memory-home/.claude.json \
  --output-format stream-json --include-hook-events --verbose \
  "Remember this synthetic discovery for the next session: PHASE6-CLEAN-OMEGA. Respond only OK."

claude -p --session-id 22222222-2222-4222-8222-222222222222 \
  --no-session-persistence --setting-sources user \
  --settings /tmp/stage6-ai-memory-home/.claude/settings.json \
  --strict-mcp-config --mcp-config /tmp/stage6-ai-memory-home/.claude.json \
  --output-format stream-json --include-hook-events --verbose \
  "Report the pending handoff discovery exactly, if present."
```

| Passagem | Resultado | Duracao | stdout/stderr |
|---|---|---:|---|
| sessao A, captura | hooks/MCP conectados; autenticacao ausente | 0,064 s | `session-a.stream.jsonl`, `session-a.stderr` |
| sessao B, handoff | marcador injetado; autenticacao ausente | 0,062 s | `session-b.stream.jsonl`, `session-b.stderr` |
| sessao com daemon parado | hook fail-open; MCP `failed` | 0,073 s | `session-offline.stream.jsonl`, `session-offline.stderr` |
| sessao depois do restart | spool drenado; MCP conectado | 0,060 s | `session-recovery.stream.jsonl`, `session-recovery.stderr` |

Nao foi coletado inventario imediatamente antes da passagem, pois o run reutilizou estado das tentativas exploratorias; essa omissao impede atribuir as contagens acumuladas somente a estas quatro sessoes. Depois da passagem, `database-final.txt` registrou 12 sessoes, 47 observacoes, 11 handoffs, 11 paginas, 32 eventos de auditoria e 47 ingest keys. Os quatro IDs sinteticos estavam encerrados; os handoffs A e offline estavam aceitos pelas sessoes B e recovery. O spool, que tinha tres JSON durante a falha, terminou somente com `.drain.lock`. Os arquivos stream e stderr foram preservados separadamente, e `context-metrics.txt` registra o volume injetado.

Na sessao A, `SessionStart` e `UserPromptSubmit` terminaram com codigo zero, o MCP `ai-memory` apareceu como `connected` e o encerramento criou a pagina `sessions/11111111-1111-4111-8111-111111111111.md` e um handoff aberto. No `SessionStart` da sessao B, o hook injetou o handoff com `PHASE6-CLEAN-OMEGA` integralmente no `additionalContext`. A injecao teve 1.232 bytes e 162 palavras. O banco registrou o handoff da sessao A como `accepted`, com `accepted_by_session` igual ao ID da sessao B, demonstrando entrega single-use entre sessoes. Como nao havia autenticacao do Claude, o modelo nao produziu a resposta solicitada; classificar a qualidade da resposta por inferencia seria indevido.

Mesmo com `--no-session-persistence` no Claude Code, ai-memory persistiu as duas sessoes. A inspecao por Python `sqlite3`, necessaria porque o CLI `sqlite3` nao esta instalado, encontrou dez sessoes, 41 observacoes, nove handoffs, nove paginas e 26 eventos de auditoria no banco acumulado de 688.128 bytes. As paginas sinteticas incluem IDs, prompts, timestamps e contagens de ferramentas; portanto a passagem confirma diretamente a falha do criterio de nao persistir sessoes.

Para testar degradacao, o daemon foi encerrado e uma terceira sessao foi iniciada. O `SessionStart` retornou codigo zero e um aviso de que o handoff nao foi injetado; Claude Code continuou com o MCP marcado como `failed`. Os hooks gravaram tres envelopes no spool, contendo ID de sessao, caminho do transcript, cwd e o prompt em claro. Depois do restart, o proximo `SessionStart` drenou os tres arquivos, recriou a pagina e o handoff da sessao offline e injetou `Offline degradation probe.`; o spool voltou a conter apenas `.drain.lock`. Assim, a falha e fail-open para Claude Code e recuperavel para captura, mas a recuperacao depende do daemon voltar e amplia a primeira persistencia sensivel anterior ao sanitizador do servidor.

## Privacidade e sessoes

O teste `crates/ai-memory-cli/tests/hook_payload.rs:80-101` exige que um payload ordinario chegue byte a byte ao spool. Assim, o sanitizador do servidor, embora tipado, nao protege a primeira persistencia. O spool pode reter entradas e bearer por varias tentativas (`crates/ai-memory-cli/src/commands/hook_spool.rs:33-44,71-89`).

O schema persiste sessoes, prompts/tool excerpts e FTS (`crates/ai-memory-store/migrations/V01__init.sql:79-104`; `V07__observations_fts_and_link_index.sql:1-32`), handoffs referenciando sessoes (`V02__handoffs.sql:6-29`) e workstreams pesquisaveis (`V31__managed_workstreams.sql:68-115`). A pagina `sessions/<id>.md` inclui session ID e titulos de prompts (`crates/ai-memory-hooks/src/synth.rs:318-395`).

## Estado atual, revisao e publicacao

Atualizacoes de pagina marcam a versao anterior como nao atual, criam `supersedes` e buscas filtram `is_latest=1` (`crates/ai-memory-store/src/ops.rs:730-808`; `reader.rs:1462-1532`). Isso e uma boa primitiva de estado atual por caminho, mas nao garante reconciliacao de conceitos distribuidos.

Auto-improve grava proposta `pending`, sidecar nao indexado e baseline para drift (`crates/ai-memory-wiki/src/wiki.rs:1120-1217`). A CLI oferece list/show/diff/approve/reject, mas nao ajuste (`crates/ai-memory-cli/src/commands/pending_writes.rs:37-159`). `require_approval=false` e o default, entao o scheduler pode aprovar imediatamente (`crates/ai-memory-cli/templates/config.default.toml:162-200`; `crates/ai-memory-consolidate/src/auto_improve_schedule.rs:332-366`). Nao ha branch, push ou pull request; commit e comando separado.

## Claude Code e testes

Ha instalador MCP, bridge por `CLAUDE_CODE_SESSION_ID` e nove hooks (`crates/ai-memory-cli/src/commands/install_mcp.rs:350-367,626-653`; `mcp_bridge.rs:19-87,118-158`; `render_shared.rs:29-49`). `SessionStart` drena o spool e injeta handoff (`hook.rs:678-715`). A Etapa 6 confirmou carregamento do bridge, captura de lifecycle, entrega single-use do handoff e replay do spool; ferramentas MCP e resposta autenticada do modelo nao foram exercitadas.

Foram encontrados testes de sanitizacao (`crates/ai-memory-core/src/sanitize.rs:348-627`), spool, rebuild (`crates/ai-memory-wiki/src/wiki.rs:4217-4307`), concorrencia/supersessao (`crates/ai-memory-store/tests/multi_session.rs:132-219`), proposta/drift (`crates/ai-memory-wiki/src/wiki.rs:2913-3098`) e relacoes contraditorias (`crates/ai-memory-consolidate/tests/typed_edges.rs:45-182`). A CI declara test, clippy, deny, audit e gitleaks (`.github/workflows/ci.yml:20-61,195-236`). Nove testes diretamente ligados a proposta, rejeicao, conflito, supersessao e idempotencia foram executados na etapa 4; os demais e a suite completa nao foram executados.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa |
|---|---|---|---|
| Markdown e Git canonicos | parcial | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | A pagina foi reconstruida, mas observacoes nao sao e o restart refez migracao, commits e arquivos canonicos. |
| Operacao local | passa | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | Captura, proposta, rejeicao, aprovacao e recuperacao concluiram sem rede externa, usando apenas providers locais. |
| Ausencia de backend obrigatorio | falha | [Dependencias](#armazenamento-e-dependencias) | Hooks e MCP integrado exigem `ai-memory serve`. |
| Indice reconstruivel | passa | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | SQLite removido e refeito da wiki preservou busca normalizada e corpo integral da pagina. |
| Privacidade antes da persistencia | falha | [Privacidade](#privacidade-e-sessoes) | O payload chega ao spool antes da sanitizacao geral. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-e-sessoes) | Prompts, observacoes, handoffs e transcripts sao duraveis. |
| Estado atual | passa | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | UPDATE substituiu o corpo e removeu o estado antigo da busca; a repeticao criou versao redundante, mas manteve apenas o estado novo autoritativo. |
| Consolidacao antes da publicacao | parcial | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | Auto-improve manteve proposta observavel antes da pagina, mas outros fluxos escrevem direto e aprovacao manual nao e padrao. |
| Revisao explicita | parcial | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | Show, diff, rejeicao e aceite funcionam; ajuste direto nao existe e requer rejeitar/regenerar. |
| Repositorio de memoria separado | passa | [Armazenamento](#armazenamento-e-dependencias) | A wiki Git e separada e contem varios projetos. |
| Integracao com Claude Code | parcial | [Etapa 6](#etapa-6-integracao-com-claude-code) | Hooks e bridge carregaram, e o handoff foi entregue; faltaram sandbox do protocolo, resposta autenticada e recuperacao completa pelas ferramentas MCP. |
| Testabilidade | passa | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | Nove contratos deterministas de proposta, hash, conflito, supersessao e idempotencia passaram offline. |
| Licenca compativel | parcial | [Licencas](../licenses.md#ai-memory) | MIT e permissiva; dependencias e ativos requerem inventario. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Mudancas estao delimitadas; custo alto a muito alto. |

## Riscos, custo e duvidas

Um fork exigiria retirar spool bruto, sessoes, handoffs e transcripts; eliminar o daemon; aceitar repositorio de memoria fornecido externamente; tornar revisao obrigatoria e ajustavel; adicionar publicacao seletiva por branch/PR; e limitar o indice ao reconstruivel. Extrair sanitizador, supersessao ou proposta ainda exige desacoplar tipos e store.

Fluxos restantes limitados:

1. Etapa 5: drift, ajuste ausente e fronteiras de commit/publicacao.
2. Etapa 6 complementar, apenas se alterar a decisao: resposta autenticada e recuperacao progressiva pelas ferramentas MCP.
3. Passagem de privacidade: confirmar sob trace o spool anterior ao sanitizador.

## Recomendacao preliminar

**Reimplementacao conceitual.** Daemon, sessoes e primeira persistencia eliminam o fork como base. Permanecem relevantes a wiki Git separada, sanitizacao tipada movida para antes do spool, supersessao, proposta com baseline/drift, reindexacao e recuperacao limitada.
