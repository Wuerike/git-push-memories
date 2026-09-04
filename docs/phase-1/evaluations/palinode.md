# Avaliacao estatica de Palinode

## Identificacao

| Campo | Valor |
|---|---|
| Projeto | Palinode |
| Repositorio | `https://github.com/phasespace-labs/palinode.git` |
| Clone local | `~/workspace/palinode` |
| Revisao | `9832d58663077645e93093bac9208a084b1d7492` |
| Branch | `main`, alinhada a `origin/main` |
| Estado inicial | arvore limpa |
| Data | 2026-09-04 |
| Avaliacao | triagem estatica, sem execucao do produto ou dos testes |
| Avaliador | OpenCode |
| Run ID | nao se aplica; nenhuma execucao comportamental |
| Commit inicial da memoria | nao se aplica; repositorios descartaveis nao foram criados |

## Hipotese

Palinode pode fornecer um componente de operacoes estruturadas, evidencias e estado epistemico, desde que essas primitivas possam ser separadas do daemon, SQLite vetorial, sessoes e LLM.

## Escopo e metodo

Foram inspecionados manifestos, API, CLI, MCP, store, save, consolidacao, plugins, hooks e testes. Nao foram executados servidor, Ollama, LLM, build ou testes.

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

Os comandos retornaram `0`; a duracao nao foi medida. A stack principal e Python 3.11+, FastAPI, Pydantic, MCP, SQLite/FTS5/sqlite-vec, YAML/frontmatter e clientes Ollama/OpenAI-compatible (`pyproject.toml:5-68`; `palinode/core/store.py:339-461`; `palinode/core/config.py:83-133`). O historico local tem 147 commits, 52 desde 2026-08-05 e multiplos contribuidores; a revisao e de 2026-09-03 e o projeto declara versao 0.15.0.

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). Somente Git e leitura estatica de arquivos foram usados.

## Fluxo estatico

| Estagio | Evidencia no clone | Conclusao |
|---|---|---|
| Entrada | `palinode/api/routers/memory.py:96-235,363-429`; `palinode/core/save.py:230-307` | REST recebe conteudo, fontes, claims e estado epistemico; CLI/MCP sao clientes HTTP. |
| Sessao | `palinode/api/routers/session.py:181-207,232-266`; `examples/hooks/palinode-session-end.sh:41-113` | SessionEnd recebe resumo, cwd, modelo, harness e session ID. |
| Filtro | `palinode/core/save.py:308-500`; `palinode/core/store.py:92-150,198-219` | Valida schema e prompt injection, mas nao detecta segredo ou PII. |
| Persistencia | `palinode/core/save.py:578-842,873-898` | Escreve Markdown, tenta commit, indexa e so depois verifica contradicao. |
| Consolidacao | `palinode/consolidation/runner.py:18-26,348-387,509-534`; `palinode/consolidation/executor.py:125-355` | LLM propoe operacoes estruturadas e executor as aplica sobre facts marcados. |
| Recuperacao | `palinode/api/routers/search.py:231-445`; `palinode/core/trace.py:283-330` | Busca hibrida e trace agregam indice, Git, fontes, claims e supersessao. |
| Reindexacao | `palinode/indexer/watcher.py:319-358` | Ha reconciliacao, mas fresh DB exige escape e pode mutar cross-refs. |

## Armazenamento e dependencias

Markdown com frontmatter e Git sao a representacao humana. `.palinode.db`, FTS, chunks, vetores e entidades sao projecoes, mas triggers, recall/ranking e logs de sessao nao sao reconstruiveis apenas dos arquivos (`palinode/core/store.py:339-461`; `PROGRAM.md:380-399`). Commits sao best-effort, e ingestao/OpenClaw possuem escritores que nao passam pelo mesmo commit (`palinode/ingest/pipeline.py:285-332`; `plugin/index.ts:1145-1169,1210-1235`).

CLI, MCP e hooks dependem de `palinode-api` (`palinode/cli/_api.py:52-176`; `palinode/mcp.py:1-9,2517-2538`). SQLite e sqlite-vec sao abertos localmente; a busca normal pode retornar 503 quando o embedder falha (`palinode/api/routers/search.py:308-320,446-449`). Consolidacao requer LLM. Portanto nao ha cloud obrigatoria em principio, mas existe backend local obrigatorio para as interfaces principais.

## Privacidade e sessoes

O scanner procura instrucoes maliciosas, nao credenciais. `/session-end` e logs de auditoria/retrieval persistem conteudo, query e session ID sem scrub geral (`palinode/core/audit.py:23-36,105-120`; `palinode/core/retrieval_log.py:31-47,103-131`). `visibility` limita leitura, nao a primeira escrita.

Uma sessao pode produzir daily log, memoria individual e status longitudinal, seguida de commit/push (`palinode/api/routers/session.py:298-435`). Em falha da API, o hook grava payload no repositorio de codigo (`examples/hooks/palinode-session-end.sh:124-138`). Esses caminhos eliminam os dois criterios de privacidade/sessao.

## Estado atual, proposta e publicacao

O vocabulario inclui `KEEP`, `UPDATE`, `MERGE`, `SUPERSEDE`, `ARCHIVE` e `RETRACT` (`specs/prompts/compaction.md:13-51`). `UPDATE` substitui um fact; `SUPERSEDE` conserva o anterior riscado; algumas operacoes nao sao idempotentes (`docs/EXECUTOR-SPEC.md:215-224`). A escolha correta para timeout/ownership ainda precisa ser executada.

`consolidate(dry_run=true)` devolve preview, mas omite campos necessarios para reaplicar exatamente a proposta (`palinode/consolidation/runner.py:774-783,869-940`). Nao ha proposal ID, ajuste, baseline ou apply-proposal. Save comum publica antes da consolidacao. Git auto-commit e padrao, push nao; staging usa arquivos explicitos (`palinode/core/config.py:357-363`; `palinode/core/git_tools.py:274-321,771-824`). Nao ha branch/PR nem drift entre preview e aplicacao.

## Claude Code e testes

O plugin marketplace configura MCP, enquanto o ciclo completo e instalado por `palinode init` (`claude-plugin/.claude-plugin/plugin.json:1-24`; `claude-plugin/.mcp.json:1-9`; `palinode/cli/init.py:1-19`). Existem hooks SessionStart, UserPromptSubmit e SessionEnd (`examples/hooks/settings.json:1-37`), ainda nao exercitados.

Ha seams para proposer e reconciliacao (`tests/test_proposer_seam.py:1-102`; `tests/test_reconcile_seam.py:1-204`), contrato do executor, testes de save/search/hooks e paridade de superficies (`tests/test_surface_parity.py:1-22`). A CI exclui `tests/integration` (`.github/workflows/ci.yml:72-74`) e a validacao L4 continua declaradamente aberta (`docs/VALIDATION-STRATEGY.md:65-74`). Nenhum teste foi executado aqui.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa estatica |
|---|---|---|---|
| Markdown e Git canonicos | parcial | [Armazenamento](#armazenamento-e-dependencias) | Commits sao best-effort e ha escritores fora do choke point. |
| Operacao local | pendente | [Dependencias](#armazenamento-e-dependencias) | Defaults podem ser locais; falta execucao offline. |
| Ausencia de backend obrigatorio | falha | [Dependencias](#armazenamento-e-dependencias) | API daemon e obrigatoria para CLI, MCP e hooks. |
| Indice reconstruivel | pendente | [Fluxo](#fluxo-estatico) | Fresh DB/reindex existe; equivalencia e mutacoes precisam ser observadas. |
| Privacidade antes da persistencia | falha | [Privacidade](#privacidade-e-sessoes) | Nao ha filtro de segredos/PII antes de save, sessao e auditoria. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-e-sessoes) | Daily, memoria, queries e session IDs sao persistidos. |
| Estado atual | pendente | [Estado atual](#estado-atual-proposta-e-publicacao) | Operacoes existem; a escolha no corpus nao foi demonstrada. |
| Consolidacao antes da publicacao | parcial | [Proposta](#estado-atual-proposta-e-publicacao) | Ha dry-run, mas save publica direto e o preview nao e aplicavel. |
| Revisao explicita | parcial | [Proposta](#estado-atual-proposta-e-publicacao) | Preview pode ser abandonado; faltam ajuste e apply da mesma proposta. |
| Repositorio de memoria separado | passa | [Armazenamento](#armazenamento-e-dependencias) | `memory_dir` e independente e comporta varios projetos. |
| Integracao com Claude Code | pendente | [Claude Code](#claude-code-e-testes) | MCP e hooks existem; falta ciclo isolado. |
| Testabilidade | pendente | [Testes](#claude-code-e-testes) | Ha seams; integracoes ficam fora da CI e nada foi executado. |
| Licenca compativel | parcial | [Licencas](../licenses.md#palinode) | Raiz/plugin sao MIT; transitivas, vetores e ativos requerem auditoria. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Mudancas foram delimitadas; produto completo custa alto. |

## Riscos, custo e duvidas

Como produto completo, o custo e alto: remover daemon/SQLite obrigatorios, filtrar antes de qualquer escrita, retirar sessoes, criar proposta duravel/ajustavel com drift e separar publicacao Git. As primitivas epistemicas tem custo baixo a medio; o executor custa medio se puder ser extraido sem store e LLM.

Fluxos restantes limitados:

1. Spike de um dia: executar parser/executor com proposer fake sobre o corpus, sem API, SQLite ou Ollama.
2. Etapa 4: comparar dry-run e aplicacao, `UPDATE`, repeticao e estado autoritativo final.
3. Etapa 3: fresh DB/reindex, hashes do Markdown e perda de estado DB-only.
4. Etapa 6: hooks e privacidade com API disponivel/indisponivel.

## Recomendacao preliminar

**Componente, condicionado ao spike de extracao.** Nao e base end-to-end. Continuam relevantes estados epistemicos, `contradicts`/`backed_by`, anchors de claims, proposer separado de executor deterministico, escrita atomica, staging explicito e reconciliacao derive/plan/apply.
