# Avaliacao estatica de agent-memory

## Identificacao

| Campo | Valor |
|---|---|
| Projeto | agent-memory |
| Repositorio | `https://github.com/xChuCx/agent-memory.git` |
| Clone local | `~/workspace/agent-memory` |
| Revisao | `51dac9bf6e72c67f6314ee3abb00515986a3fa4a` |
| Branch | `main`, alinhada a `origin/main` |
| Estado inicial | arvore limpa |
| Data | 2026-09-04 |
| Avaliacao | triagem estatica, sem execucao do produto ou dos testes |
| Avaliador | OpenCode |
| Run ID | nao se aplica; nenhuma execucao comportamental |
| Commit inicial da memoria | nao se aplica; repositorios descartaveis nao foram criados |

## Hipotese

agent-memory pode fornecer o nucleo de operacoes, diff e aplicacao explicita, desde que memoria e codigo possam ter roots distintos e as lacunas de privacidade e drift sejam corrigiveis sem reescrever o componente.

## Escopo e metodo

Foram inspecionados CLI, MCP, schema, update/staging, scanner, indice, Git, adaptador Claude e testes. Nao foram executados build, comandos ou testes.

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

Os comandos retornaram `0`; a duracao nao foi medida. O projeto usa Go 1.25, Cobra, MCP Go SDK, Goldmark, YAML, locks e SQLite/FTS5 puro Go (`go.mod:1-12`). O historico local tem 97 commits, nenhum desde 2026-08-05 e autoria efetivamente concentrada em uma identidade; a revisao/tag 0.5.1 e de 2026-06-09.

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). Somente Git e leitura estatica de arquivos foram usados.

## Fluxo estatico

| Estagio | Evidencia no clone | Conclusao |
|---|---|---|
| Entrada | `internal/cli/propose.go:59-153`; `internal/mcp/propose.go:92-157` | CLI/MCP recebem operacoes estruturadas. |
| Planejamento | `internal/memory/update.go:173-198,226-459` | Valida categoria/schema, aplica operacoes em memoria e gera post-state. |
| Scan | `internal/memory/secrets.go:73-180`; `internal/memory/pii.go:20-58`; `internal/memory/update.go:462-490` | Scan cobre formatos conhecidos e o Markdown final, nao todos os metadados. |
| Staging | `internal/memory/update.go:751-839` | Persiste request, checksums e copias integrais propostas. |
| Revisao | `internal/cli/review.go:104-157`; `internal/cli/diff.go:80-165` | Exibe diff; reject e apply sao comandos separados. |
| Aplicacao | `internal/memory/staging.go:215-314,410-496` | Verifica drift conforme politica e sobrescreve arquivos staged. |
| Recuperacao | `internal/memory/fetch.go:160-193`; `internal/index/query.go:40-72`; `internal/index/rebuild.go:28-48` | FTS5 e reconstruido do Markdown. |

## Armazenamento e roots

Markdown em `.agent-memory/` e canonico; staging, sessions e logs sao locais; SQLite e indice shadow (`agent-memory-design-doc-v0.4.1.md:292-330`; `internal/index/sqlite.go:20-35`). O rebuild pode inserir IDs no Markdown (`internal/index/rebuild.go:137-163`), portanto ainda requer teste de nao alteracao indevida.

O risco decisivo e o root: `--root` e raiz do projeto e a memoria e sempre `<root>/.agent-memory` (`internal/cli/helpers.go:13-30`). Branch/changed files e auto-stage usam o mesmo repositorio (`internal/cli/fetch.go:116-150`; `internal/memory/update.go:656-660`). `manifest.project.root` nao e consumido. Stores federados sao read-only (`internal/config/stores.go:26-29`). Logo, o requisito de repositorio de memoria separado falha sem uma mudanca transversal.

Nao ha backend remoto ou daemon permanente. MCP e stdio; SQLite e embutido (`internal/mcp/server.go:71-76`).

## Privacidade, sessoes e drift

Os marcadores sinteticos do protocolo nao correspondem aos prefixos secretos nem aos defaults de PII: o segredo fica abaixo do limiar de entropia e e-mail vem desabilitado. Apenas o post-state Markdown e escaneado; `rationale`, `sources`, owner e outros metadados entram em `proposal.json` sem scan (`internal/memory/update.go:811-827`). Lock info pode receber owner/op antes do scan (`internal/lock/lock.go:107-118`). `apply` nao reescaneia nem revalida staging (`internal/memory/staging.go:410-424`).

O layout persiste `sessions/*.md`, owner/session refs, propostas e sidecars, contrariando a politica comum de nao persistir material ou IDs de sessao.

Algumas operacoes usam precondicoes fracas, como arquivo presente ou secao resolvivel (`internal/memory/operations.go:475-481,548-553`). Como apply grava o snapshot integral, uma mudanca concorrente fora da secao pode ser perdida. Aplicacao multi-arquivo tambem nao e transacional.

## Estado atual, governanca e publicacao

`replace_section`, `replace_section_content`, archive e delete fornecem boas primitivas (`internal/memory/operations.go:347-419,570-626,755-913`), mas o agente precisa identificar a contradicao. Append pode manter dois estados autoritativos.

Review permite inspecao/diff, reject e apply. Nao ha comando seguro para ajustar. Editar staging manualmente e inseguro porque apply nao verifica o post-state nem repete scan/schema. Categorias duraveis normalmente stageiam, mas pitfalls podem auto-aplicar e `--apply` contorna revisao separada.

Git add usa caminhos explicitos, mas commit inclui qualquer alteracao previamente staged (`internal/git/commit.go:10-86`). Nao ha branch, push ou PR. O merge driver por IDs continua relevante (`internal/markdown/merge.go:8-30`).

## Claude Code e testes

O adaptador instala skill e `.mcp.json`; a skill instrui fetch no inicio e propose no fim (`internal/adapters/claude/SKILL.md:23-69`; `internal/adapters/claude/claude.go:71-181`). Nao ha hooks; o modelo precisa seguir a instrucao. O ciclo real nao foi executado.

Foram encontrados 83 arquivos de teste. A CI declara build/vet/test em tres sistemas, race, E2E e lint (`.github/workflows/ci.yml:8-86`). O E2E percorre init, MCP, propose, review, apply, fetch, segredo e archive (`internal/e2e/release01_smoke_test.go:30-68,103-621`). Ha testes de rebuild e drift (`internal/cli/rebuild_index_test.go:32-151`; `internal/memory/rebase_test.go:213-315`), mas nao cobrem roots separados, tampering, drift fora da secao ou staged alheio. Nada foi executado nesta etapa.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa estatica |
|---|---|---|---|
| Markdown e Git canonicos | passa | [Armazenamento](#armazenamento-e-roots) | Markdown e duravel; SQLite/cache sao derivados. |
| Operacao local | pendente | [Armazenamento](#armazenamento-e-roots) | A arquitetura e local, mas falta executar o fluxo offline. |
| Ausencia de backend obrigatorio | passa | [Armazenamento](#armazenamento-e-roots) | MCP stdio e transitorio; nao ha daemon. |
| Indice reconstruivel | pendente | [Fluxo](#fluxo-estatico) | Implementacao existe; corpus/rebuild limpo nao foram executados. |
| Privacidade antes da persistencia | falha | [Privacidade](#privacidade-sessoes-e-drift) | Marcadores e metadados escapam do scan. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-sessoes-e-drift) | Sessions, IDs e metadados sao persistidos localmente. |
| Estado atual | pendente | [Estado atual](#estado-atual-governanca-e-publicacao) | Replace existe; consolidacao requer execucao. |
| Consolidacao antes da publicacao | parcial | [Governanca](#estado-atual-governanca-e-publicacao) | Ha staging, mas auto-apply e `--apply` permitem mutacao direta. |
| Revisao explicita | parcial | [Governanca](#estado-atual-governanca-e-publicacao) | Diff/reject/apply existem; falta ajuste seguro. |
| Repositorio de memoria separado | falha | [Roots](#armazenamento-e-roots) | Memoria e fixa em `<code_root>/.agent-memory`. |
| Integracao com Claude Code | pendente | [Claude Code](#claude-code-e-testes) | Skill/MCP existem, sem ciclo real demonstrado. |
| Testabilidade | pendente | [Testes](#claude-code-e-testes) | Suite ampla nao executada e faltam casos criticos. |
| Licenca compativel | parcial | [Licencas](../licenses.md#agent-memory) | Apache-2.0; transitivas/notices e npm requerem auditoria. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Separar roots tem custo e risco altos delimitados. |

## Riscos, custo e duvidas

Separar roots exige propagar `CodeRoot`, `MemoryRoot` e repositorio Git, namespear por repositorio consumidor, adaptar MCP/doctor/proveniencia e testar dois code repos concorrentes. O hotfix 0.5.1 corrige roteamento cruzado anterior (`CHANGELOG.md:8-23`), o que aumenta o risco dessa mudanca.

Fluxos restantes limitados:

1. Passagem de privacidade em conteudo e metadados, seguida de adulteracao do staging.
2. Spike de um dia: `CODE_ROOT != MEMORY_ROOT`, dois repositorios consumidores e Git somente na memoria.
3. Etapa 5: politicas de drift e commit com indice previamente staged.
4. Etapa 3/4: rebuild, processo novo, edicao externa, replace e idempotencia.

## Recomendacao preliminar

**Componente, condicionado ao spike de roots e ao endurecimento de privacidade/apply.** Continuam relevantes o motor Markdown byte-preserving, operacoes por secao, FTS5, staging/diff, lock, merge driver e proveniencia.
