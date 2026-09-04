# Avaliacao estatica de Basic Memory

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
| Avaliacao | triagem estatica, sem execucao do produto ou dos testes |
| Avaliador | OpenCode |
| Run ID | nao se aplica; nenhuma execucao comportamental |
| Commit inicial da memoria | nao se aplica; repositorios descartaveis nao foram criados |

## Hipotese

Basic Memory pode informar o formato Markdown, a indexacao local e a recuperacao, mas a AGPL, a captura de sessoes e as mutacoes diretas provavelmente o eliminam como base ou componente do produto Apache-2.0.

## Escopo e metodo

Foram inspecionados manifestos, documentacao, implementacao e testes das fronteiras de escrita, indexacao, hooks e busca. Build, instalacao, hooks, testes e fluxos com o corpus comum ficaram excluidos desta etapa.

O ponto fixo foi confirmado com os comandos abaixo, todos com codigo `0`. A duracao nao foi medida porque nao houve fluxo comportamental.

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

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). Somente Git e leitura estatica de arquivos foram usados.

## Fluxo estatico

| Estagio | Evidencia no clone | Conclusao |
|---|---|---|
| Entrada MCP | `src/basic_memory/mcp/tools/write_note.py:197-226,352-435`; `src/basic_memory/mcp/tools/edit_note.py:348-445,585-761` | `write_note` e `edit_note` recebem conteudo e chamam mutacoes da API diretamente. |
| Entrada por arquivo | `src/basic_memory/index/local_project.py:222-287`; `src/basic_memory/index/local_dependencies.py:493-677` | Scanner e watcher reconciliam Markdown alterado externamente. |
| Filtro | `src/basic_memory/ignore_utils.py:19-68,156-283`; `src/basic_memory/markdown/entity_parser.py:274-287` | Ha filtro de caminhos, mas o conteudo so perde BOM/NUL; nao ha scan de segredo ou PII. |
| Persistencia | `src/basic_memory/indexing/accepted_note_mutation_runner.py:692-782`; `src/basic_memory/models/knowledge.py:182-241`; `src/basic_memory/index/note_content_materialization.py:623-759` | O fluxo MCP grava primeiro a copia integral e as projecoes no banco, depois materializa o Markdown. |
| Estado atual | `src/basic_memory/services/note_preparation.py:555-592,644-690`; `src/basic_memory/repository/observation_repository.py:122-180` | `find_replace` e `replace_section` substituem bytes e projecoes, mas nao detectam contradicao semanticamente. |
| Recuperacao | `src/basic_memory/services/search_service.py:216-287,406-466`; `src/basic_memory/mcp/tools/read_note.py:169-465`; `src/basic_memory/services/context_service.py:110-445` | Busca FTS/vetorial, leitura direta e travessia de grafo dependem do indice local. |
| Reindexacao | `src/basic_memory/cli/commands/db.py:190-269,307-436`; `src/basic_memory/indexing/project_index_coordinator.py:245-343` | Existe reset/reindex a partir dos arquivos, ainda nao executado com o corpus comum. |

## Armazenamento e dependencias

O Markdown e legivel e portavel, mas o banco nao e somente indice: `NoteContent` guarda o Markdown integral e `AcceptedProjectNoteChange` guarda um journal nao reconstruivel (`docs/DOMAIN_MODEL.md:126-159,191-210`; `src/basic_memory/models/project.py:123-174`). Escritas aceitas podem existir no banco antes da materializacao. Git e uma sincronizacao externa/manual, nao um mecanismo de publicacao do produto (`README.md:162-174`).

O fluxo local minimo pode usar SQLite e transporte MCP stdio sem Postgres, Redis, Milvus ou servidor HTTP separado. O cliente usa `ASGITransport` no processo (`src/basic_memory/mcp/async_client.py:62-73,224-238`). Busca semantica, auto-update e os launchers `uv` adicionam dependencias que devem ser neutralizadas no run offline (`src/basic_memory/config_models.py:64-69,251-263,714-722`; `src/basic_memory/cli/commands/mcp.py:85-100`).

## Governanca, privacidade e Claude Code

`PreparedEntityWrite` e apenas um objeto interno; nao pode ser inspecionado, ajustado, rejeitado e aceito por uma pessoa (`src/basic_memory/services/note_preparation.py:80-117`). As operacoes MCP persistem imediatamente e nao ha branch, commit ou pull request. A confirmacao de compartilhamento em `plugins/claude-code/skills/bm-share/SKILL.md:27-51` cobre uma copia especifica, nao as mutacoes comuns.

O plugin registra `SessionStart` e `PreCompact` (`plugins/claude-code/hooks/hooks.json:1-29`). `PreCompact` le o transcript e grava uma nota com solicitacoes humanas, `claude_session_id` e metadados da sessao (`src/basic_memory/cli/commands/hook.py:754-797,894-1004,1055-1123`). Envelopes de hook com identificador de sessao tambem ficam em inbox persistente (`src/basic_memory/hooks/envelope.py:47-79`; `src/basic_memory/hooks/inbox.py:1-12,73-115`).

Nao ha descarte de segredo antes de `write_note`, `NoteContent`, FTS ou Markdown. O teste `tests/cli/test_hook_command.py:785-808` inclusive preserva uma cadeia com aparencia de credencial em checkpoint. Isso basta para eliminar estaticamente os criterios de privacidade e de ausencia de sessoes.

## Testes e contratos encontrados

Ha testes focados para escrita/leitura (`tests/mcp/test_tool_write_note.py:153-296`), edicao estruturada (`tests/mcp/test_tool_edit_note.py:79-266`), atomicidade e remocao de busca antiga (`tests/api/v2/test_accepted_note_atomicity.py:141-251`), materializacao (`tests/runtime/test_note_content_store_materialization.py:83-140`), reindexacao (`tests/cli/test_db_reindex.py:161-247,526-655`) e hooks (`tests/cli/test_hook_command.py:140-385,636-808`). Eles nao foram executados nesta etapa.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa estatica |
|---|---|---|---|
| Markdown e Git canonicos | parcial | [Armazenamento](#armazenamento-e-dependencias) | Ha autoridade DB-first temporaria, journal nao derivado e Git externo. |
| Operacao local | pendente | [Dependencias](#armazenamento-e-dependencias) | A arquitetura admite SQLite/FTS local; o fluxo offline ainda precisa ser executado. |
| Ausencia de backend obrigatorio | passa | [Dependencias](#armazenamento-e-dependencias) | API ASGI e watcher operam no mesmo processo; backends externos sao opcionais. |
| Indice reconstruivel | pendente | [Fluxo](#fluxo-estatico) | O comando existe, mas equivalencia da recuperacao ainda nao foi demonstrada. |
| Privacidade antes da persistencia | falha | [Privacidade](#governanca-privacidade-e-claude-code) | Nao existe filtro de conteudo antes das escritas. |
| Sem persistencia de sessoes | falha | [Claude Code](#governanca-privacidade-e-claude-code) | Checkpoints, trechos de transcript e IDs de sessao sao persistidos. |
| Estado atual | parcial | [Fluxo](#fluxo-estatico) | Ha substituicao estrutural, sem consolidacao semantica. |
| Consolidacao antes da publicacao | falha | [Governanca](#governanca-privacidade-e-claude-code) | A preparacao interna nao e proposta observavel e a mutacao e direta. |
| Revisao explicita | falha | [Governanca](#governanca-privacidade-e-claude-code) | Nao ha inspecao, ajuste, rejeicao e aceite. |
| Repositorio de memoria separado | passa | [Armazenamento](#armazenamento-e-dependencias) | O projeto de memoria e configuravel e pode servir varios repositorios. |
| Integracao com Claude Code | pendente | [Claude Code](#governanca-privacidade-e-claude-code) | Plugin e hooks existem, mas o ciclo isolado nao foi executado. |
| Testabilidade | pendente | [Testes](#testes-e-contratos-encontrados) | Ha contratos extensos, ainda nao executados. |
| Licenca compativel | falha | [Licencas](../licenses.md#basic-memory) | AGPL impede fork/componente Apache-2.0; referencia independente continua permitida. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Mudancas estao delimitadas; adocao direta e inviavel sem relicenciamento. |

## Riscos, custo e duvidas

Como base, exigiria criar privacidade pre-persistencia, proposta/revisao, publicacao Git e consolidacao, alem de remover a persistencia de sessoes. O custo tecnico e alto e a AGPL impede reaproveitar diretamente o nucleo.

A unica duvida capaz de alterar reutilizacao direta e o escopo da afirmacao MIT em `plugins/claude-code/README.md:216-218`, pois nao existe `LICENSE` propria no subdiretorio. Ela so merece spike se houver intencao concreta de reutilizar o plugin.

Fluxos restantes limitados:

1. Etapa 3: FTS local, edicao externa, remocao/rebuild e comparacao da consulta e do Git diff de frontmatter.
2. Etapa 4: substituicao exata versus append para timeout e ownership, incluindo idempotencia.
3. Etapa 6: `SessionStart`/`PreCompact` sob trace e falha do MCP, com `BM_BIN` fixo na revisao.

## Recomendacao preliminar

**Referencia.** Fork e componente estao eliminados pela AGPL; privacidade, sessoes e governanca eliminam a arquitetura como base. Permanecem relevantes Markdown estruturado, reconciliacao arquivo-indice, FTS local, CAS/checksums, edicao por secao e recuperacao limitada no `SessionStart`.
