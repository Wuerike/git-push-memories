# Avaliacao de Basic Memory

## Identificacao

| Campo | Valor |
|---|---|
| Projeto | Basic Memory |
| Repositorio | `https://github.com/basicmachines-co/basic-memory.git` |
| Clone local | `~/workspace/basic-memory` |
| Revisao | `a55003e7d0ab36161fa5e933198e3f62907160e7` |
| Branch | `main`, alinhada a `origin/main` |
| Estado inicial | arvore limpa |
| Data | 2026-09-04 |
| Avaliacao | triagem estatica e etapa 3 comportamental; hooks, LLM e testes nao executados |
| Avaliador | OpenCode |
| Run ID | `20260904T211353Z-basic-memory-stage3` |
| Commit inicial da memoria | `ff7a04745e2a5ed9d1602383965834112dea5ee4` |

## Hipotese

Basic Memory pode informar o formato Markdown, a indexacao local e a recuperacao, mas a AGPL, a captura de sessoes e as mutacoes diretas provavelmente o eliminam como base ou componente do produto Apache-2.0.

## Escopo e metodo

Foram inspecionados manifestos, documentacao, implementacao e testes das fronteiras de escrita, indexacao, hooks e busca. Na etapa 3, o produto foi instalado e executado com o corpus comum para avaliar FTS local, edicao externa e rebuild. Hooks, LLM e testes permaneceram excluidos.

O ponto fixo foi confirmado com os comandos abaixo, todos com codigo `0`. A duracao dessa confirmacao estatica nao foi medida; as duracoes do fluxo comportamental aparecem na secao da etapa 3.

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

O projeto usa Python 3.12+, FastAPI/FastMCP, SQLAlchemy, SQLite/FTS5, `sqlite-vec`, Typer e Pydantic (`pyproject.toml:1-52,84-118`). O historico local tem 1.995 commits, 367 desde 2026-08-05 e multiplos contribuidores; a revisao e de 2026-09-04. Isso demonstra atividade recente no ponto fixo, sem afirmar atividade posterior.

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). O run comportamental usou Bubblewrap 0.6.1, `uv` 0.6.17 e Python 3.12.10. O pacote construido da revisao foi `0.23.3.dev216+a55003e7`, enquanto `bm --version` informou 0.23.2; a divergencia de metadados fica registrada. Dependencias foram provisionadas com rede e o trust bundle do sistema montado somente para leitura; build e fluxo avaliado ocorreram sem rede e sem variaveis de proxy.

## Fluxo estatico

| Estagio | Evidencia no clone | Conclusao |
|---|---|---|
| Entrada MCP | `src/basic_memory/mcp/tools/write_note.py:197-226,352-435`; `src/basic_memory/mcp/tools/edit_note.py:348-445,585-761` | `write_note` e `edit_note` recebem conteudo e chamam mutacoes da API diretamente. |
| Entrada por arquivo | `src/basic_memory/index/local_project.py:222-287`; `src/basic_memory/index/local_dependencies.py:493-677` | Scanner e watcher reconciliam Markdown alterado externamente. |
| Filtro | `src/basic_memory/ignore_utils.py:19-68,156-283`; `src/basic_memory/markdown/entity_parser.py:274-287` | Ha filtro de caminhos, mas o conteudo so perde BOM/NUL; nao ha scan de segredo ou PII. |
| Persistencia | `src/basic_memory/indexing/accepted_note_mutation_runner.py:692-782`; `src/basic_memory/models/knowledge.py:182-241`; `src/basic_memory/index/note_content_materialization.py:623-759` | O fluxo MCP grava primeiro a copia integral e as projecoes no banco, depois materializa o Markdown. |
| Estado atual | `src/basic_memory/services/note_preparation.py:555-592,644-690`; `src/basic_memory/repository/observation_repository.py:122-180` | `find_replace` e `replace_section` substituem bytes e projecoes, mas nao detectam contradicao semanticamente. |
| Recuperacao | `src/basic_memory/services/search_service.py:216-287,406-466`; `src/basic_memory/mcp/tools/read_note.py:169-465`; `src/basic_memory/services/context_service.py:110-445` | Busca FTS/vetorial, leitura direta e travessia de grafo dependem do indice local. |
| Reindexacao | `src/basic_memory/cli/commands/db.py:190-269,307-436`; `src/basic_memory/indexing/project_index_coordinator.py:245-343` | Existe reset/reindex a partir dos arquivos; a etapa 3 demonstrou o fluxo com o corpus comum. |

## Armazenamento e dependencias

O Markdown e legivel e portavel, mas o banco nao e somente indice: `NoteContent` guarda o Markdown integral e `AcceptedProjectNoteChange` guarda um journal nao reconstruivel (`docs/DOMAIN_MODEL.md:126-159,191-210`; `src/basic_memory/models/project.py:123-174`). Escritas aceitas podem existir no banco antes da materializacao. Git e uma sincronizacao externa/manual, nao um mecanismo de publicacao do produto (`README.md:162-174`).

O fluxo local minimo pode usar SQLite e transporte MCP stdio sem Postgres, Redis, Milvus ou servidor HTTP separado. O cliente usa `ASGITransport` no processo (`src/basic_memory/mcp/async_client.py:62-73,224-238`). Busca semantica, auto-update e os launchers `uv` adicionam dependencias que devem ser neutralizadas no run offline (`src/basic_memory/config_models.py:64-69,251-263,714-722`; `src/basic_memory/cli/commands/mcp.py:85-100`).

## Etapa 3: armazenamento e recuperacao

O run bruto foi preservado em `.artifacts/phase-1/20260904T211353Z-basic-memory-stage3-evidence`, conforme o [manifesto de evidencias](../evidence-manifest.md). A copia executavel permaneceu destacada na revisao fixa e terminou limpa; o repositorio de codigo sintetico tambem terminou limpo. O projeto `main` apontou para `/workspace/memory`, separado de `/workspace/code`. Busca semantica, watcher, auto-update, promocoes, telemetria e reescrita de frontmatter foram desabilitados explicitamente.

Configuracao efetiva relevante, aplicada a todos os comandos do produto:

```text
BASIC_MEMORY_CONFIG_DIR=/home/phase-1/basic-memory-state
BASIC_MEMORY_HOME=/workspace/memory
BASIC_MEMORY_DATABASE_BACKEND=sqlite
BASIC_MEMORY_FORCE_LOCAL=true
BASIC_MEMORY_INDEX_CHANGES=false
BASIC_MEMORY_SEMANTIC_SEARCH_ENABLED=false
BASIC_MEMORY_DEFAULT_SEARCH_TYPE=text
BASIC_MEMORY_RERANKER_ENABLED=false
BASIC_MEMORY_ENSURE_FRONTMATTER_ON_SYNC=false
BASIC_MEMORY_AUTO_UPDATE=false
BASIC_MEMORY_NO_PROMOS=1
BASIC_MEMORY_LOGFIRE_ENABLED=false
BASIC_MEMORY_LOGFIRE_SEND_TO_LOGFIRE=false
BASIC_MEMORY_LOG_LEVEL=WARNING
LOGFIRE_IGNORE_NO_CONFIG=1
```

O subfluxo principal da etapa 3, sempre por `run_isolated`, foi:

```bash
bm reindex --full --search --project main
bm tool search-notes "Qual e o timeout de checkout-api e qual time e responsavel?" --project main --local --json
bm tool read-note checkout-api --project main --local --json
# pelo host, a partir da raiz do git-push-memories:
cp docs/phase-1/corpus/memory-expected/services/checkout-api.md "$MEMORY_ROOT/services/checkout-api.md"
bm reindex --search --project main
bm tool search-notes "Qual e o timeout de checkout-api e qual time e responsavel?" --project main --local --json
bm tool read-note checkout-api --project main --local --json
printf 'y\n' | bm reset --force
bm reindex --full --search --project main
bm tool search-notes "Qual e o timeout de checkout-api e qual time e responsavel?" --project main --local --json
bm tool read-note checkout-api --project main --local --json
```

O helper `run_isolated` e a preparacao dos tres repositorios seguiram literalmente [`evaluation-protocol.md`](../evaluation-protocol.md#preparacao-dos-repositorios). Os checks `config list`, `project list`, `project info` e `status` tambem retornaram `0`, mas nao foram cronometrados. Os JSON completos e os arquivos de evidencia nomeados abaixo estao no `EVIDENCE_ROOT` indicado.

| Rotulo de evidencia | Codigo | Duracao | stdout | stderr |
|---|---:|---:|---|---|
| `initial-index` | 0 | 2,22 s | 1 observado/1 indexado | vazio |
| `query-initial` | 0 | 5,29 s | 1 resultado; 53 palavras/587 bytes | vazio |
| `read-initial` | 0 | 5,24 s | cinco segundos/`team-a`; 27 palavras/251 bytes | vazio |
| `external-edit-index` | 0 | 2,24 s | 1 observado/1 indexado | vazio |
| `query-after-edit` | 0 | 6,97 s | 1 resultado; 53 palavras/586 bytes | vazio |
| `read-after-edit` | 0 | 6,69 s | tres segundos/`team-b`; 27 palavras/250 bytes | vazio |
| `reset-index` | 0 | 3,19 s | `Database reset complete` | vazio |
| `rebuild-from-markdown` | 0 | 2,44 s | 1 observado/1 indexado | vazio |
| `query-after-rebuild` | 0 | 6,03 s | 1 resultado; 53 palavras/586 bytes | vazio |
| `read-after-rebuild` | 0 | 6,02 s | tres segundos/`team-b`; 27 palavras/250 bytes | vazio |

`retrieval-comparison.json` confirma busca normalizada e conteudo integral iguais antes/depois do rebuild, com ausencia do estado antigo. Os inventarios `before-reset-db.json`, `after-reset-db.json` e `after-rebuild-db.json` mostram respectivamente 1, 0 e 1 linha em `entity`, `note_content` e `search_index`.

O hash final do Markdown (`a25761aae03007249bd8d85a2764c2c54421d114`) e igual ao hash do fixture esperado. O unico diff no repositorio de memoria e a troca de cinco segundos/`team-a` por tres segundos/`team-b`; a ferramenta nao adicionou frontmatter. Fora do repositorio, persistiu `config.json` (3.318 bytes), `.bmignore` (659 bytes), log vazio e `memory.db` (286.720 bytes) com SHM (32.768 bytes) e WAL vazio. O banco reconstruido tem uma copia integral em `note_content` e uma entrada FTS, mas nenhuma linha no journal `accepted_project_note_change`. A comparacao com o inventario pos-build tambem registrou bytecode `__pycache__` sob a `.venv`; sao caches de execucao no clone descartavel, cuja arvore Git permaneceu limpa.

As latencias incluem inicializacao fria de um novo processo Python e ASGI por comando. Elas demonstram conclusao offline, mas nao sao comparaveis diretamente com um daemon residente.

## Governanca, privacidade e Claude Code

`PreparedEntityWrite` e apenas um objeto interno; nao pode ser inspecionado, ajustado, rejeitado e aceito por uma pessoa (`src/basic_memory/services/note_preparation.py:80-117`). As operacoes MCP persistem imediatamente e nao ha branch, commit ou pull request. A confirmacao de compartilhamento em `plugins/claude-code/skills/bm-share/SKILL.md:27-51` cobre uma copia especifica, nao as mutacoes comuns.

O plugin registra `SessionStart` e `PreCompact` (`plugins/claude-code/hooks/hooks.json:1-29`). `PreCompact` le o transcript e grava uma nota com solicitacoes humanas, `claude_session_id` e metadados da sessao (`src/basic_memory/cli/commands/hook.py:754-797,894-1004,1055-1123`). Envelopes de hook com identificador de sessao tambem ficam em inbox persistente (`src/basic_memory/hooks/envelope.py:47-79`; `src/basic_memory/hooks/inbox.py:1-12,73-115`).

Nao ha descarte de segredo antes de `write_note`, `NoteContent`, FTS ou Markdown. O teste `tests/cli/test_hook_command.py:785-808` inclusive preserva uma cadeia com aparencia de credencial em checkpoint. Isso basta para eliminar estaticamente os criterios de privacidade e de ausencia de sessoes.

## Testes e contratos encontrados

Ha testes focados para escrita/leitura (`tests/mcp/test_tool_write_note.py:153-296`), edicao estruturada (`tests/mcp/test_tool_edit_note.py:79-266`), atomicidade e remocao de busca antiga (`tests/api/v2/test_accepted_note_atomicity.py:141-251`), materializacao (`tests/runtime/test_note_content_store_materialization.py:83-140`), reindexacao (`tests/cli/test_db_reindex.py:161-247,526-655`) e hooks (`tests/cli/test_hook_command.py:140-385,636-808`). Eles nao foram executados na triagem nem no run comportamental.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa |
|---|---|---|---|
| Markdown e Git canonicos | parcial | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | A pagina e a busca foram reconstruidas, mas ha autoridade DB-first temporaria, journal nao derivado e Git externo. |
| Operacao local | parcial | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | Indexacao, busca, edicao externa e rebuild concluiram offline; captura e consolidacao nao foram executadas. |
| Ausencia de backend obrigatorio | passa | [Dependencias](#armazenamento-e-dependencias) | API ASGI e watcher operam no mesmo processo; backends externos sao opcionais. |
| Indice reconstruivel | passa | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | Banco zerado e reconstruido do Markdown preservou busca normalizada e leitura integral. |
| Privacidade antes da persistencia | falha | [Privacidade](#governanca-privacidade-e-claude-code) | Nao existe filtro de conteudo antes das escritas. |
| Sem persistencia de sessoes | falha | [Claude Code](#governanca-privacidade-e-claude-code) | Checkpoints, trechos de transcript e IDs de sessao sao persistidos. |
| Estado atual | parcial | [Etapa 3](#etapa-3-armazenamento-e-recuperacao) | Edicao externa removeu o estado antigo da busca, mas falta avaliar consolidacao semantica. |
| Consolidacao antes da publicacao | falha | [Governanca](#governanca-privacidade-e-claude-code) | A preparacao interna nao e proposta observavel e a mutacao e direta. |
| Revisao explicita | falha | [Governanca](#governanca-privacidade-e-claude-code) | Nao ha inspecao, ajuste, rejeicao e aceite. |
| Repositorio de memoria separado | passa | [Armazenamento](#armazenamento-e-dependencias) | O projeto de memoria e configuravel e pode servir varios repositorios. |
| Integracao com Claude Code | não se aplica | [Claude Code](#governanca-privacidade-e-claude-code) | O ciclo nao foi executado; hooks e launchers foram excluidos da referencia final por licenca e acoplamento ao nucleo AGPL. |
| Testabilidade | não se aplica | [Testes](#testes-e-contratos-encontrados) | A suite nao foi executada; a AGPL eliminou reutilizacao direta e tornou o fluxo nao decisivo. |
| Licenca compativel | falha | [Licencas](../licenses.md#basic-memory) | AGPL impede fork/componente Apache-2.0; referencia independente continua permitida. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Mudancas estao delimitadas; adocao direta e inviavel sem relicenciamento. |

## Riscos, custo e duvidas

Como base, exigiria criar privacidade pre-persistencia, proposta/revisao, publicacao Git e consolidacao, alem de remover a persistencia de sessoes. O custo tecnico e alto e a AGPL impede reaproveitar diretamente o nucleo.

A unica duvida capaz de alterar reutilizacao direta e o escopo da afirmacao MIT em `plugins/claude-code/README.md:216-218`, pois nao existe `LICENSE` propria no subdiretorio. Ela so merece spike se houver intencao concreta de reutilizar o plugin.

Fluxos dispensados por nao alterarem a disposicao:

1. Consolidacao semantica e idempotencia nao foram executadas; a edicao externa da Etapa 3 prova apenas reconciliacao arquivo-indice.
2. `SessionStart`/`PreCompact` e a suite fonte nao foram executados e nao sustentam classificacao comportamental positiva.

## Recomendacao preliminar

**Referencia.** Fork e componente estao eliminados pela AGPL; privacidade, sessoes e governanca eliminam a arquitetura como base. Permanecem relevantes Markdown estruturado, reconciliacao arquivo-indice, FTS local, CAS/checksums e edicao por secao. Plugin, hooks e comportamento de `SessionStart` ficaram fora da referencia final.
