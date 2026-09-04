# Avaliacao estatica de ai-memory

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
| Avaliacao | triagem estatica, sem execucao do produto ou dos testes |
| Avaliador | OpenCode |
| Run ID | nao se aplica; nenhuma execucao comportamental |
| Commit inicial da memoria | nao se aplica; repositorios descartaveis nao foram criados |

## Hipotese

ai-memory e o comparador end-to-end mais proximo, mas daemon, spool anterior a sanitizacao e persistencia de sessoes podem elimina-lo como base, preservando conceitos de wiki, supersessao e proposta.

## Escopo e metodo

Foram lidos manifestos, migracoes, CLI, hooks, store, wiki, consolidacao, MCP e testes. Nao foram executados build, daemon, hooks, LLM ou testes.

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

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). Somente Git e leitura estatica de arquivos foram usados.

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
| Reindexacao | `crates/ai-memory-cli/src/commands/reindex.rs:29-82`; `crates/ai-memory-wiki/src/wiki.rs:1354-1431` | Reconstroi paginas, links, FTS e entidades, nao sessoes e auditoria. |

## Armazenamento e dependencias

A wiki Markdown fica em um repositorio Git proprio dentro do data dir (`crates/ai-memory-wiki/src/wiki.rs:80-135`). Paginas sao canonicas, mas SQLite tambem guarda sessoes, observacoes, handoffs, propostas e auditoria que nao podem ser reconstruidos da wiki. As propostas sao explicitamente DB-authoritative (`crates/ai-memory-store/migrations/V21__auto_improve_pending_proposals.sql:1-45`). `commit_all` adiciona toda a arvore da wiki (`crates/ai-memory-wiki/src/git.rs:69-129`), o que exigira isolamento antes de qualquer reutilizacao.

O fluxo de hooks e MCP HTTP requer `ai-memory serve`, que abre banco, watcher e schedulers (`crates/ai-memory-cli/src/commands/serve.rs:619-807,920-1205`). SQLite e Git sao embutidos, e operacao sem cloud e possivel em principio. Consolidacao e review exigem um LLM local ou remoto; MCP stdio sozinho nao recebe hooks.

## Privacidade e sessoes

O teste `crates/ai-memory-cli/tests/hook_payload.rs:80-101` exige que um payload ordinario chegue byte a byte ao spool. Assim, o sanitizador do servidor, embora tipado, nao protege a primeira persistencia. O spool pode reter entradas e bearer por varias tentativas (`crates/ai-memory-cli/src/commands/hook_spool.rs:33-44,71-89`).

O schema persiste sessoes, prompts/tool excerpts e FTS (`crates/ai-memory-store/migrations/V01__init.sql:79-104`; `V07__observations_fts_and_link_index.sql:1-32`), handoffs referenciando sessoes (`V02__handoffs.sql:6-29`) e workstreams pesquisaveis (`V31__managed_workstreams.sql:68-115`). A pagina `sessions/<id>.md` inclui session ID e titulos de prompts (`crates/ai-memory-hooks/src/synth.rs:318-395`).

## Estado atual, revisao e publicacao

Atualizacoes de pagina marcam a versao anterior como nao atual, criam `supersedes` e buscas filtram `is_latest=1` (`crates/ai-memory-store/src/ops.rs:730-808`; `reader.rs:1462-1532`). Isso e uma boa primitiva de estado atual por caminho, mas nao garante reconciliacao de conceitos distribuidos.

Auto-improve grava proposta `pending`, sidecar nao indexado e baseline para drift (`crates/ai-memory-wiki/src/wiki.rs:1120-1217`). A CLI oferece list/show/diff/approve/reject, mas nao ajuste (`crates/ai-memory-cli/src/commands/pending_writes.rs:37-159`). `require_approval=false` e o default, entao o scheduler pode aprovar imediatamente (`crates/ai-memory-cli/templates/config.default.toml:162-200`; `crates/ai-memory-consolidate/src/auto_improve_schedule.rs:332-366`). Nao ha branch, push ou pull request; commit e comando separado.

## Claude Code e testes

Ha instalador MCP, bridge por `CLAUDE_CODE_SESSION_ID` e nove hooks (`crates/ai-memory-cli/src/commands/install_mcp.rs:350-367,626-653`; `mcp_bridge.rs:19-87,118-158`; `render_shared.rs:29-49`). `SessionStart` drena o spool e injeta handoff (`hook.rs:678-715`). O ciclo nao foi executado.

Foram encontrados testes de sanitizacao (`crates/ai-memory-core/src/sanitize.rs:348-627`), spool, rebuild (`crates/ai-memory-wiki/src/wiki.rs:4217-4307`), concorrencia/supersessao (`crates/ai-memory-store/tests/multi_session.rs:132-219`), proposta/drift (`crates/ai-memory-wiki/src/wiki.rs:2913-3098`) e relacoes contraditorias (`crates/ai-memory-consolidate/tests/typed_edges.rs:45-182`). A CI declara test, clippy, deny, audit e gitleaks (`.github/workflows/ci.yml:20-61,195-236`), sem resultado reproduzido nesta etapa.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa estatica |
|---|---|---|---|
| Markdown e Git canonicos | parcial | [Armazenamento](#armazenamento-e-dependencias) | A recuperacao tambem usa observacoes nao reconstruiveis da wiki. |
| Operacao local | pendente | [Dependencias](#armazenamento-e-dependencias) | Loopback e LLM local sao possiveis; falta execucao offline. |
| Ausencia de backend obrigatorio | falha | [Dependencias](#armazenamento-e-dependencias) | Hooks e MCP integrado exigem `ai-memory serve`. |
| Indice reconstruivel | pendente | [Fluxo](#fluxo-estatico) | Implementacao existe; equivalencia com o corpus nao foi demonstrada. |
| Privacidade antes da persistencia | falha | [Privacidade](#privacidade-e-sessoes) | O payload chega ao spool antes da sanitizacao geral. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-e-sessoes) | Prompts, observacoes, handoffs e transcripts sao duraveis. |
| Estado atual | parcial | [Estado atual](#estado-atual-revisao-e-publicacao) | Ha supersessao por pagina, sem garantia geral entre conceitos. |
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

1. Etapa 3: consulta, remocao do SQLite, reindexacao da wiki e nova consulta.
2. Etapa 4: `require_approval=true`, update/reject/approve/idempotencia e drift no corpus comum.
3. Etapa 6: hooks reais em configuracao isolada, medindo handoff e inventariando sessoes.
4. Passagem de privacidade: confirmar sob trace o spool anterior ao sanitizador.

## Recomendacao preliminar

**Reimplementacao conceitual.** Daemon, sessoes e primeira persistencia eliminam o fork como base. Permanecem relevantes a wiki Git separada, sanitizacao tipada movida para antes do spool, supersessao, proposta com baseline/drift, reindexacao e recuperacao limitada.
