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
| Data | 2026-09-04 |
| Avaliacao | triagem estatica e etapa 3 comportamental; hooks, LLM e testes nao executados |
| Avaliador | OpenCode |
| Run ID | `20260904T212935Z-ai-memory-stage3` |
| Commit inicial da memoria | `29645b91101fbf98d524c8766d83cd4ae1ca3367` |

## Hipotese

ai-memory e o comparador end-to-end mais proximo, mas daemon, spool anterior a sanitizacao e persistencia de sessoes podem elimina-lo como base, preservando conceitos de wiki, supersessao e proposta.

## Escopo e metodo

Foram lidos manifestos, migracoes, CLI, hooks, store, wiki, consolidacao, MCP e testes. Na etapa 3, o binario foi compilado e o daemon foi executado com o corpus comum para avaliar FTS, edicao externa e rebuild. Hooks, LLM e testes permaneceram excluidos.

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

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). O run comportamental usou Bubblewrap 0.6.1, Rust/Cargo 1.95.0 e ai-memory 1.39.0. Dependencias foram provisionadas com rede e o trust bundle do sistema montado somente para leitura; o build de 405,44 s e todo o fluxo avaliado ocorreram sem rede e sem variaveis de proxy.

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

## Privacidade e sessoes

O teste `crates/ai-memory-cli/tests/hook_payload.rs:80-101` exige que um payload ordinario chegue byte a byte ao spool. Assim, o sanitizador do servidor, embora tipado, nao protege a primeira persistencia. O spool pode reter entradas e bearer por varias tentativas (`crates/ai-memory-cli/src/commands/hook_spool.rs:33-44,71-89`).

O schema persiste sessoes, prompts/tool excerpts e FTS (`crates/ai-memory-store/migrations/V01__init.sql:79-104`; `V07__observations_fts_and_link_index.sql:1-32`), handoffs referenciando sessoes (`V02__handoffs.sql:6-29`) e workstreams pesquisaveis (`V31__managed_workstreams.sql:68-115`). A pagina `sessions/<id>.md` inclui session ID e titulos de prompts (`crates/ai-memory-hooks/src/synth.rs:318-395`).

## Estado atual, revisao e publicacao

Atualizacoes de pagina marcam a versao anterior como nao atual, criam `supersedes` e buscas filtram `is_latest=1` (`crates/ai-memory-store/src/ops.rs:730-808`; `reader.rs:1462-1532`). Isso e uma boa primitiva de estado atual por caminho, mas nao garante reconciliacao de conceitos distribuidos.

Auto-improve grava proposta `pending`, sidecar nao indexado e baseline para drift (`crates/ai-memory-wiki/src/wiki.rs:1120-1217`). A CLI oferece list/show/diff/approve/reject, mas nao ajuste (`crates/ai-memory-cli/src/commands/pending_writes.rs:37-159`). `require_approval=false` e o default, entao o scheduler pode aprovar imediatamente (`crates/ai-memory-cli/templates/config.default.toml:162-200`; `crates/ai-memory-consolidate/src/auto_improve_schedule.rs:332-366`). Nao ha branch, push ou pull request; commit e comando separado.

## Claude Code e testes

Ha instalador MCP, bridge por `CLAUDE_CODE_SESSION_ID` e nove hooks (`crates/ai-memory-cli/src/commands/install_mcp.rs:350-367,626-653`; `mcp_bridge.rs:19-87,118-158`; `render_shared.rs:29-49`). `SessionStart` drena o spool e injeta handoff (`hook.rs:678-715`). O ciclo nao foi executado.

Foram encontrados testes de sanitizacao (`crates/ai-memory-core/src/sanitize.rs:348-627`), spool, rebuild (`crates/ai-memory-wiki/src/wiki.rs:4217-4307`), concorrencia/supersessao (`crates/ai-memory-store/tests/multi_session.rs:132-219`), proposta/drift (`crates/ai-memory-wiki/src/wiki.rs:2913-3098`) e relacoes contraditorias (`crates/ai-memory-consolidate/tests/typed_edges.rs:45-182`). A CI declara test, clippy, deny, audit e gitleaks (`.github/workflows/ci.yml:20-61,195-236`), mas esses testes nao foram executados na triagem nem no run comportamental.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa |
|---|---|---|---|
| Markdown e Git canonicos | parcial | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | A pagina foi reconstruida, mas observacoes nao sao e o restart refez migracao, commits e arquivos canonicos. |
| Operacao local | pendente | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | Recuperacao concluiu sem rede e sem LLM; captura e consolidacao do fluxo principal ainda nao foram executadas. |
| Ausencia de backend obrigatorio | falha | [Dependencias](#armazenamento-e-dependencias) | Hooks e MCP integrado exigem `ai-memory serve`. |
| Indice reconstruivel | passa | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | SQLite removido e refeito da wiki preservou busca normalizada e corpo integral da pagina. |
| Privacidade antes da persistencia | falha | [Privacidade](#privacidade-e-sessoes) | O payload chega ao spool antes da sanitizacao geral. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-e-sessoes) | Prompts, observacoes, handoffs e transcripts sao duraveis. |
| Estado atual | parcial | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | Edicao externa removeu o estado antigo e rebuild reteve so a versao atual, sem garantir reconciliacao entre conceitos. |
| Consolidacao antes da publicacao | parcial | [Revisao](#estado-atual-revisao-e-publicacao) | Auto-improve propoe; outros fluxos escrevem direto. |
| Revisao explicita | parcial | [Revisao](#estado-atual-revisao-e-publicacao) | Diff, rejeicao e aceite existem; ajuste nao e autoaprovacao e padrao. |
| Repositorio de memoria separado | passa | [Armazenamento](#armazenamento-e-dependencias) | A wiki Git e separada e contem varios projetos. |
| Integracao com Claude Code | pendente | [Claude Code](#claude-code-e-testes) | Hooks e bridge existem; falta o ciclo isolado. |
| Testabilidade | pendente | [Testes](#claude-code-e-testes) | Ha contratos fortes e CI, ainda nao executados. |
| Licenca compativel | parcial | [Licencas](../licenses.md#ai-memory) | MIT e permissiva; dependencias e ativos requerem inventario. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Mudancas estao delimitadas; custo alto a muito alto. |

## Riscos, custo e duvidas

Um fork exigiria retirar spool bruto, sessoes, handoffs e transcripts; eliminar o daemon; aceitar repositorio de memoria fornecido externamente; tornar revisao obrigatoria e ajustavel; adicionar publicacao seletiva por branch/PR; e limitar o indice ao reconstruivel. Extrair sanitizador, supersessao ou proposta ainda exige desacoplar tipos e store.

Fluxos restantes limitados:

1. Etapa 4: `require_approval=true`, update/reject/approve/idempotencia e drift no corpus comum.
2. Etapa 6: hooks reais em configuracao isolada, medindo handoff e inventariando sessoes.
3. Passagem de privacidade: confirmar sob trace o spool anterior ao sanitizador.

## Recomendacao preliminar

**Reimplementacao conceitual.** Daemon, sessoes e primeira persistencia eliminam o fork como base. Permanecem relevantes a wiki Git separada, sanitizacao tipada movida para antes do spool, supersessao, proposta com baseline/drift, reindexacao e recuperacao limitada.
