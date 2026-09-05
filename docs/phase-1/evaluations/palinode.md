# Avaliacao de Palinode

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
| Avaliacao | triagem estatica e etapa 4 comportamental; hooks, privacidade e rebuild nao executados |
| Avaliador | OpenCode |
| Run ID | `20260904T234427Z-palinode-stage4` |
| Commit inicial da memoria | `5f2a2f04e5fd2c28b4d37d2e4734c77d7246ffbf` |

## Hipotese

Palinode pode fornecer um componente de operacoes estruturadas, evidencias e estado epistemico, desde que essas primitivas possam ser separadas do daemon, SQLite vetorial, sessoes e LLM.

## Escopo e metodo

Foram inspecionados manifestos, API, CLI, MCP, store, save, consolidacao, plugins, hooks e testes. Na etapa 4, API, CLI, consolidacao, Git, busca e testes focados foram executados com providers deterministas em loopback. Hooks, privacidade e rebuild permaneceram excluidos.

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

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). O run comportamental usou Bubblewrap 0.6.1, `uv` 0.6.17, Python 3.11.12 e Palinode 0.15.0. Dependencias foram provisionadas com rede; build, fluxo, providers locais e testes executaram sem rede externa ou variaveis de proxy.

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

O vocabulario inclui `KEEP`, `UPDATE`, `MERGE`, `SUPERSEDE`, `ARCHIVE` e `RETRACT` (`specs/prompts/compaction.md:13-51`). `UPDATE` substitui um fact; `SUPERSEDE` conserva o anterior riscado; algumas operacoes nao sao idempotentes (`docs/EXECUTOR-SPEC.md:215-224`). A etapa 4 executou `UPDATE` para timeout/ownership e registrou abaixo por que isso nao garante a escolha de um modelo real.

`consolidate(dry_run=true)` devolve preview, mas omite campos necessarios para reaplicar exatamente a proposta (`palinode/consolidation/runner.py:774-783,869-940`). Nao ha proposal ID, ajuste, baseline ou apply-proposal. Save comum publica antes da consolidacao. Git auto-commit e padrao, push nao; staging usa arquivos explicitos (`palinode/core/config.py:357-363`; `palinode/core/git_tools.py:274-321,771-824`). Nao ha branch/PR nem drift entre preview e aplicacao.

## Etapa 4: consolidacao do estado atual

O run bruto valido foi preservado em `.artifacts/phase-1/20260904T234427Z-palinode-stage4-evidence`, conforme o [manifesto de evidencias](../evidence-manifest.md); configuracao e scripts completos de preparacao, mock, fluxo e replay estao no subdiretorio `harness/`. Para repetir, copie os cinco arquivos para `/tmp/opencode/` e execute `bash /tmp/opencode/run-palinode-stage4.sh`. Duas tentativas anteriores foram descartadas apenas para classificacao de testes: a primeira nao instalou o extra `dev`; a segunda desabilitou instrumentation e, por isso, contradisse um teste de trace. O run valido repetiu todo o fluxo do produto e terminou com repositorios de memoria, codigo e candidato limpos.

Palinode nao reconhece `services/checkout-api.md` como alvo de consolidacao. O run moveu o mesmo corpo para `projects/checkout-api.md`, registrou essa adaptacao em commit e usou `bootstrap-ids` para criar os IDs estaveis `checkout-api-0c366f` e `checkout-api-36d8e1`. O prompt de compaction tambem precisou ser copiado para `specs/prompts/` dentro do repositorio de memoria, pois ele nao e empacotado no wheel.

Configuracao efetiva relevante:

```yaml
embeddings:
  primary:
    model: phase1-mock-embed
    url: http://127.0.0.1:49375
    dimensions: 64
auto_summary:
  enabled: false
context:
  enabled: false
consolidation:
  llm_url: http://127.0.0.1:49375
  llm_model: phase1-mock-consolidator
  llm_temperature: 0.0
  allowed_ops: [KEEP, UPDATE]
  write_time:
    enabled: false
services:
  api:
    host: 127.0.0.1
    port: 49374
git:
  auto_commit: true
  auto_push: false
audit:
  enabled: false
instrumentation:
  capture_retrievals: false
```

O ambiente tambem definiu `PALINODE_ALLOW_FRESH_DB=1`, exigido porque a memoria inicial existe antes do SQLite descartavel.

O provider OpenAI-compatible devolveu duas operacoes `UPDATE` deterministicas, com IDs, novo texto e razao baseados em `service.yaml`. O provider Ollama-compatible devolveu vetores lexicais deterministas de 64 dimensoes. Eles comprovam o contrato e a operacao local, nao a qualidade de um modelo real.

```bash
palinode save --type ProjectSnapshot --slug checkout-api \
  --update-policy replace --epistemic fact --source phase-1-seed \
  --file "$MEMORY_ROOT/projects/checkout-api.md" --format json
palinode bootstrap-ids --format json
palinode reindex --format json
palinode save --type ResearchRef --slug checkout-api-service \
  --epistemic fact --source phase-1-code \
  --file "$CODE_ROOT/services/checkout-api/service.yaml" --format json
palinode dedup-suggest \
  --content 'checkout-api usa timeout de tres segundos e agora pertence ao team-b' \
  --min-similarity 0 --top-k 5 --format json
palinode save --type Insight --slug checkout-api-discovery \
  --project checkout-api --epistemic fact --source phase-1-discovery \
  --cite 'research/checkout-api-service.md::timeout_seconds: 3' \
  --cite 'research/checkout-api-service.md::owner: team-b' \
  --claim 'O timeout de requisicoes e de tres segundos.::research/checkout-api-service.md::timeout_seconds: 3' \
  --claim 'O servico pertence ao `team-b`.::research/checkout-api-service.md::owner: team-b' \
  --backed-by research/checkout-api-service --format json \
  'checkout-api usa timeout de tres segundos e agora pertence ao team-b'
palinode trace insights/checkout-api-discovery.md --format json
palinode consolidate --dry-run --source insights --format json
palinode consolidate --source insights --format json
palinode reindex --format json
palinode read projects/checkout-api.md --meta --format json
palinode search --limit 10 --threshold 0 --no-context --format json \
  'Qual e o timeout de checkout-api e qual time e responsavel?'
palinode consolidate --source insights --format json
# replay adicional das mesmas duas operacoes pela funcao apply_operations
python /opt/palinode-stage4-replay.py
```

| Rotulo de evidencia | Codigo | Duracao | stdout | stderr |
|---|---:|---:|---|---|
| `seed-page` / `bootstrap-ids` / `reindex-initial` | 0 | 0,49/0,38/0,43 s | 249/38/164 bytes | diagnostico de config |
| `read-initial` / `query-initial` | 0 | 0,38/0,40 s | cinco segundos/`team-a`; 1.081/1.404 bytes | diagnostico de config |
| `save-evidence` / `dedup-before-discovery` / `save-discovery` | 0 | 0,40/0,39/0,43 s | 273/628/279 bytes | diagnostico de config |
| `trace-discovery` / `blame-discovery` | 0 | 0,41/0,43 s | 2.708/3.575 bytes | diagnostico de config |
| `consolidation-preview` | 0 | 0,48 s | 2 updates resumidos; 484 bytes | diagnostico de config |
| `consolidation-apply` | 0 | 0,43 s | 2 updates e 1 nota arquivada; 372 bytes | diagnostico de config |
| `reindex-after-apply` | 0 | 0,47 s | 4 arquivos/7 chunks FTS; 164 bytes | diagnostico de config |
| `read-after-apply` | 0 | 0,47 s | tres segundos/`team-b`; 1.385 bytes | diagnostico de config |
| `query-after-apply` | 0 | 0,53 s | 7 resultados; 12.304 bytes | diagnostico de config |
| buscas de estado antigo/novo | 0 | 0,46/0,59 s | 12.500/12.582 bytes | diagnostico de config |
| `trace-target` / `history-target` | 0 | 0,66/0,49 s | 2.124/4.493 bytes | diagnostico de config |
| `consolidation-repeat` | 0 | 0,63 s | nenhuma nota; 51 bytes | diagnostico de config |
| `executor-exact-replay` | 0 | 0,27 s | bytes estaveis; 208 bytes | config e 2 avisos `UPDATE unmatched` |
| `focused-tests` | 0 | 9,58 s | 130 testes passaram em 8,08 s | vazio |

O preview manteve o hash `58bc70264df2c8be04a4d273732e746bebf07bbf` e o HEAD `e743f13431c654a14acae02ba9677608dbecda25`, mas sua interface publica exibiu somente tipo, arquivo e rationale. IDs e novos textos existiam apenas na resposta capturada pelo harness. A aplicacao fez uma segunda chamada ao provider; `mock-requests.jsonl` confirma duas requisicoes semanticamente iguais. Portanto nao existe aceite da mesma proposta inspecionada.

A aplicacao trocou os dois fatos sem conservar cinco segundos ou `team-a` no arquivo atual, adicionou um `Consolidation Log`, arquivou a descoberta e criou o commit `3033d73d4c0a5cf1c30b6b9c58bca106f71d8279`. O hash final foi `b700bdd8abab19662fd41794a0615c3587528162`. O segundo comando publico nao encontrou notas porque a descoberta ja estava arquivada. O replay exato dos dois `UPDATE` preservou os bytes, embora o executor tenha contabilizado os dois no campo `unmatched`, que mistura IDs ausentes com substituicoes ja identicas.

O `trace` da descoberta registrou duas claims com hashes de quote validos e `backed_by`; o `trace` do alvo final nao recebeu sources, claims ou `backed_by`. A proveniencia detalhada permanece na descoberta arquivada, enquanto o alvo recebe apenas razoes textuais no log e historico Git. Os gaps G2 (extracao por fato) e G3 (acao consumidora) continuaram explicitos.

Antes do save da descoberta, `dedup-suggest` encontrou o alvo inicial com similaridade 0,7282 e a evidencia com 0,5262, ambos sem `strong_dup`; o resultado foi apenas consultivo. Nenhum link ou objeto de contradicao foi criado antes do `UPDATE`, e o target final continuou com `contradicts: none`. Assim, a substituicao foi correta porque o mock escolheu `UPDATE`, nao porque o pipeline classificou formalmente a contradicao ou vinculou a evidencia ao fato atualizado.

O prompt operacional copiado para `specs/prompts/compaction.md` entrou no proprio indice. Com threshold zero, a consulta final retornou a descoberta arquivada, o alvo, a evidencia e quatro chunks do prompt, totalizando 12.304 bytes. A busca pelo estado antigo tambem retornou sete vizinhos, mas nenhum continha os valores antigos; isso demonstra ausencia textual, nao precisao de ranking.

O inventario final registrou `.palinode.db` de 425.984 bytes, SHM de 32.768 bytes, log de operacoes de 14.676 bytes, 7 chunks/FTS e um repositorio Git limpo com oito commits. Os 130 testes focados cobriram IDs, proposer seam, dry-run, executor, replace guard, sources, claims e trace.

## Claude Code e testes

O plugin marketplace configura MCP, enquanto o ciclo completo e instalado por `palinode init` (`claude-plugin/.claude-plugin/plugin.json:1-24`; `claude-plugin/.mcp.json:1-9`; `palinode/cli/init.py:1-19`). Existem hooks SessionStart, UserPromptSubmit e SessionEnd (`examples/hooks/settings.json:1-37`), ainda nao exercitados.

Ha seams para proposer e reconciliacao (`tests/test_proposer_seam.py:1-102`; `tests/test_reconcile_seam.py:1-204`), contrato do executor, testes de save/search/hooks e paridade de superficies (`tests/test_surface_parity.py:1-22`). A CI exclui `tests/integration` (`.github/workflows/ci.yml:72-74`) e a validacao L4 continua declaradamente aberta (`docs/VALIDATION-STRATEGY.md:65-74`). A etapa 4 executou 130 testes focados; integracoes live e a suite completa permaneceram excluidas.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa |
|---|---|---|---|
| Markdown e Git canonicos | parcial | [Armazenamento](#armazenamento-e-dependencias) | Commits sao best-effort e ha escritores fora do choke point. |
| Operacao local | passa | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | Save, preview, aplicacao, Git e recuperacao concluiram sem rede externa, com providers locais. |
| Ausencia de backend obrigatorio | falha | [Dependencias](#armazenamento-e-dependencias) | API daemon e obrigatoria para CLI, MCP e hooks. |
| Indice reconstruivel | pendente | [Fluxo](#fluxo-estatico) | Fresh DB/reindex existe; equivalencia e mutacoes precisam ser observadas. |
| Privacidade antes da persistencia | falha | [Privacidade](#privacidade-e-sessoes) | Nao ha filtro de segredos/PII antes de save, sessao e auditoria. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-e-sessoes) | Daily, memoria, queries e session IDs sao persistidos. |
| Estado atual | parcial | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | `UPDATE` produziu estado atual e replay estavel, mas a escolha depende do proposer e o prompt normal recomenda `SUPERSEDE`, nao idempotente. |
| Consolidacao antes da publicacao | parcial | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | Dry-run preservou o alvo, mas omitiu a operacao exata e apply consultou o modelo novamente. |
| Revisao explicita | parcial | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | Preview pode ser abandonado; faltam ajuste, rejeicao registrada e aceite da mesma proposta. |
| Repositorio de memoria separado | passa | [Armazenamento](#armazenamento-e-dependencias) | `memory_dir` e independente e comporta varios projetos. |
| Integracao com Claude Code | pendente | [Claude Code](#claude-code-e-testes) | MCP e hooks existem; falta ciclo isolado. |
| Testabilidade | passa | [Etapa 4](#etapa-4-consolidacao-do-estado-atual) | 130 testes deterministas de consolidacao, executor e proveniencia passaram offline. |
| Licenca compativel | parcial | [Licencas](../licenses.md#palinode) | Raiz/plugin sao MIT; transitivas, vetores e ativos requerem auditoria. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Mudancas foram delimitadas; produto completo custa alto. |

## Riscos, custo e duvidas

Como produto completo, o custo e alto: remover daemon/SQLite obrigatorios, filtrar antes de qualquer escrita, retirar sessoes, criar proposta duravel/ajustavel com drift e separar publicacao Git. As primitivas epistemicas tem custo baixo a medio; o executor custa medio se puder ser extraido sem store e LLM.

Fluxos restantes limitados:

1. Spike de um dia: extrair parser/executor sem API, SQLite ou Ollama; o proposer seam e o replay da etapa 4 reduzem, mas nao encerram, essa duvida de acoplamento.
2. Etapa 3: fresh DB/reindex, hashes do Markdown e perda de estado DB-only.
3. Etapa 5: drift e proposta aplicavel; a etapa 4 confirmou que o preview publico nao basta.
4. Etapa 6: hooks e privacidade com API disponivel/indisponivel.

## Recomendacao preliminar

**Componente, condicionado ao spike de extracao.** Nao e base end-to-end. Continuam relevantes estados epistemicos, `contradicts`/`backed_by`, anchors de claims, proposer separado de executor deterministico, escrita atomica, staging explicito e reconciliacao derive/plan/apply.
