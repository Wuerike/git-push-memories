# Promote Simplification Tasks

## Execution Protocol

Implementar com a skill `tlc-spec-driven`, seguindo o ciclo de execução, gates e commits atômicos. Ativar a skill pelo nome; não substituir seu protocolo por este resumo. Se indisponível, parar e informar.

**Design**: `.specs/features/promote-simplification/design.md`
**Status**: In Progress
**Approved**: 2026-11-03 - user: "aprovado"
**Mutations**: 3
**Build**: go vet ./...; go build ./...; go test -race ./...

A especificação e as tarefas estão aprovadas; a abordagem A foi aceita com "aceito A". A execução proposta pelo principal foi aprovada; não houve pedido para delegar trabalhadores. Aprovação autoriza mudanças e commits locais; não autoriza publicar este repositório de desenvolvimento.

## Test Coverage Matrix

> Generated from codebase, project guidelines, and spec. Guidelines found: `Makefile`, `.github/workflows/ci.yml`, `README.md` (Desenvolvimento) e `tests/e2e/run-phase-3.sh`. Não há AGENTS.md ou CONTRIBUTING no projeto. Aplicar cobertura de todos os critérios do escopo e dos três limites simples da spec; não criar cenários que o usuário excluiu.

Amostras lidas: testes de workspace, inbox, memory/page_rewrite, memory/validation, release, app, mcpserver/server_inbox, claudeadapter/promote_create e e2e/phase3. Usam `testing`, fixtures Git locais e fake `gh`. Não há linter separado; o verificador estático configurado é `go vet`. Usar `gofmt` nos arquivos Go alterados.

| Code Layer | Required Test Type | Coverage Expectation | Location Pattern | Run Command |
| --- | --- | --- | --- | --- |
| Formato canônico | unit | IDs/bytes esperados, repetição da conferência, estrutura existente e exemplos inválidos | `internal/memory/*test.go` | `go test ./internal/memory` |
| Workspace e núcleo de consolidação | integration | Todos os critérios programáticos e erros simples especificados; remotos locais e GitHub simulado | `internal/workspace/*test.go`, `internal/consolidation/*test.go` | `go test ./internal/workspace ./internal/consolidation` |
| Transporte MCP e CLI | integration/e2e | Todas as entradas novas, rejeições básicas, preservação de captura/leitura e percurso completo | `internal/mcpserver/*test.go`, `internal/app/*test.go`, `tests/e2e/*test.go` | `go test ./internal/mcpserver ./internal/app ./tests/e2e` |
| Skill e documentação | contract | Uma prova nomeada por obrigação escrita; testes não alegam executar o julgamento de um LLM | `tests/claudeadapter/*test.go` | `go test ./tests/claudeadapter` |
| Remoção do motor antigo | regression/integration | Suite suportada continua verde; remoções de testes só dos contratos retirados explicitamente | suites existentes preservadas | `go test ./...` |

## Gate Check Commands

> Generated from codebase. Go 1.25.12 é a toolchain fixada no go.mod/Makefile; disponibilizar esse Go no PATH antes da primeira execução. Ele não está no PATH desta sessão de planejamento. Não há testes da aplicação executados nesta etapa.

| Gate Level | When to Use | Command |
| --- | --- | --- |
| Quick | Mudança unitária ou helper Git isolado | `go test ./internal/memory` ou `go test ./internal/workspace`, conforme a tarefa |
| Full | Núcleo, transporte, CLI, contratos e remoção | `go test ./...` |
| Build | Fechamento das fases e tarefa final | `make verify`: testes, race, vet e build com Go 1.25.12 |

Cada tarefa inclui seus testes no mesmo commit. As contagens mínimas abaixo são de testes novos/atualizados planejados, não resultados já obtidos; registrar totais reais após os gates. A redução da contagem legada exige inventário dos testes de funcionalidades explicitamente removidas. Nenhum teste de comportamento preservado pode ser enfraquecido ou apagado.

## Execution Plan

### Phase 1: Documents and preparation

Executar T1, T2, T3 e T4 nessa ordem.

```
T1 → T4
T2 → T3 → T4
```

### Phase 2: Publication and MCP

Executar T5, T6 e T7 nessa ordem, após a fase 1.

```
T4 → T5 → T6 → T7
```

### Phase 3: Skill and removal

Executar T8, T9 e T10 nessa ordem, após a fase 2.

```
T7 → T8 → T9 → T10
```

### Phase 4: Usage documentation

Executar T11, T12, T13 e T14 nessa ordem, após a fase 3.

```
T10 → T11 → T12 → T13 → T14
```

## Task Breakdown

### T1: Disponibilizar preparação de documentos canônicos

**What**: Expor a preparação de um documento completo para edição direta, reutilizando as regras de IDs e estrutura atuais.
**Where**: `internal/memory/document.go`
**Depends on**: None
**Reuses**: parser Markdown, injectSectionIDs e validateDocument
**Requirement**: PROM-15, PROM-16

**Done when**:

- [x] PrepareDocument preserva IDs mantidos, completa headings novos e aceita documento já preparado sem mudar bytes válidos.
- [x] Os testes verificam os IDs esperados e os bytes, sem chamar preview ou reconcile.
- [x] Os testes novos e a suíte existente de memória passam; registrar contagem real.
- [x] Gate da tarefa passa: `go test ./internal/memory`.

**Tests**: unit; testes em internal/memory/document_test.go, pelo menos 4 testes nomeados para IDs preservados/novos, repetição da conferência, frontmatter e documento inválido.
**Gate**: Quick — go test ./internal/memory
**Commit**: `feat(memory): prepare canonical documents for direct editing`

**Result**: Complete — memory gate: 139 testes/subtestes passaram, incluindo 4 testes novos nomeados; zero falhas. Go 1.25.12 instalado e dependências disponíveis em /tmp/promote-tools.

### T2: Preparar caminhos e worktree de promoção

**What**: Adicionar o layout de promoção e o helper que cria branch/worktree novos a partir de uma revisão explícita.
**Where**: `internal/workspace/workspace.go`
**Depends on**: None
**Reuses**: PathsFor, Git.Output e identidade/layout existentes
**Requirement**: PROM-01, PROM-22, PROM-29

**Done when**:

- [x] NewPromotion cria branch promote/<UTC>-<random> em uma revisão explícita e retorna worktree e inputs separados.
- [x] Paths centraliza Promotions e Published; não faz push, merge, reset ou remoção de trabalho anterior.
- [x] Testes conferem HEAD e os caminhos absolutos; registrar contagem real.
- [x] Gate da tarefa passa: `go test ./internal/workspace`.

**Tests**: integration; ampliar internal/workspace/workspace_test.go com pelo menos 2 testes de base exata e caminhos de insumos/comprovante fora do worktree.
**Gate**: Quick — go test ./internal/workspace
**Commit**: `feat(workspace): prepare isolated promotion worktrees`

**Result**: Complete — workspace gate: 8 testes/subtestes passaram, incluindo 2 testes novos; zero falhas.

### T3: Capturar propostas fixas e inventário de memórias

**What**: Implementar Manager e Prepare com revisões remotas fixas, cópia integral das propostas e inventário canônico.
**Where**: `internal/consolidation/prepare.go`
**Depends on**: T2
**Reuses**: workspace.Fetch/NewPromotion, padrão ls-tree/cat-file e nomes de inbox
**Requirement**: PROM-01, PROM-02, PROM-04, PROM-05, PROM-29, PROM-32, PROM-34

**Done when**:

- [x] A preparação lê revisões capturadas após fetch e devolve inventário sem cortar propostas ou truncar conteúdo.
- [x] Os insumos copiados ficam fora do worktree publicável; uma proposta recebida depois não altera esse conjunto.
- [x] Erros interrompem a preparação; ausência de propostas não cria PR ou commit; nenhuma escrita remota abre um lock.
- [x] A suíte completa permanece verde; registrar contagens dos novos testes e regressões.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: integration; internal/consolidation/prepare_test.go com pelo menos 6 testes selecionáveis de base, bytes integrais, 17 propostas, chegada posterior, ausência de propostas e falha de fetch/leitura.
**Gate**: Full — go test ./...
**Commit**: `feat(consolidation): capture fixed proposal inputs`

**Result**: Complete — 6 testes novos de preparação passaram; gate completo go test ./... -timeout=120s passou em todos os pacotes. Fixture ajustada antes do commit sem alterar asserções. Nenhuma publicação remota.

### T4: Conferir arquivos atuais e produzir diff

**What**: Implementar Check para completar IDs, validar arquivos canônicos editados e mostrar o diff atual, incluindo arquivos novos.
**Where**: `internal/consolidation/check.go`
**Depends on**: T1, T3
**Reuses**: memory.PrepareDocument, config.IsMemoryPath e Git diff
**Requirement**: PROM-15, PROM-16, PROM-17, PROM-19, PROM-21, PROM-32

**Done when**:

- [x] A conferência não cria commit nem token e inclui arquivos novos no diff.
- [x] IDs são preparados antes da revisão final; documentos ou categorias inválidos impedem publicação.
- [x] Arquivos auxiliares não são tratados como memória; edições humanas aparecem nos bytes/diff correntes.
- [x] Testes de conferência e suíte completa passam; registrar contagem real.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: integration; internal/consolidation/check_test.go com pelo menos 3 testes de formato/categorias, arquivo novo e edição humana atual.
**Gate**: Full — go test ./...
**Commit**: `feat(consolidation): check current memory files`

**Result**: Complete — 3 testes novos nomeados (6 testes/subtestes), gate completo e make verify passaram: testes comuns, race, vet e build. Nenhum teste existente removido.

### T5: Publicar um commit global e abrir PR

**What**: Implementar Submit com Markdown livre de resumo, commit restrito às memórias atuais, push e comprovante de publicação.
**Where**: `internal/consolidation/submit.go`
**Depends on**: T4
**Reuses**: workspace.Commit/PushCAS e github.PRCreate/PRFind
**Requirement**: PROM-08, PROM-19, PROM-22, PROM-23, PROM-24, PROM-29, PROM-32

**Done when**:

- [x] Submit valida sem modificar os arquivos aprovados e cria um único commit apenas dos caminhos canônicos alterados.
- [x] Push abre PR para main e retorna o link; o resumo inclui IDs/destinos/justificativas como texto livre, sem decisões por proposta.
- [x] Somente após criar o PR é salvo Published fora do worktree, sem plano ou sessão serializada.
- [x] A falha de push/PR é visível e não apaga trabalho ou declara sucesso; sem retry/rollback.
- [x] Testes conferem árvore Git, bytes, requests ao fake gh e comprovante; registrar contagem real.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: integration; internal/consolidation/submit_test.go com pelo menos 5 testes de escopo do commit, edição humana, PR/base main, resumo livre e erro de publicação.
**Gate**: Full — go test ./...
**Commit**: `feat(consolidation): publish reviewed memory files`

**Result**: Complete — 6 testes novos nomeados de publicação e gate completo passaram; comprovante mínimo fora do worktree, bytes humanos preservados e PR GitHub simulado.

### T6: Limpar propostas concluídas após merge

**What**: Implementar Finish a partir do comprovante publicado, confirmando merge e removendo apenas propostas idênticas às aprovadas.
**Where**: `internal/consolidation/finish.go`
**Depends on**: T5
**Reuses**: github.PRView, workspace.Fetch/Commit/PushCAS/UpdateRead e lock do checkout compartilhado
**Requirement**: PROM-25, PROM-26, PROM-27, PROM-32

**Done when**:

- [x] Antes da confirmação do merge não ocorre limpeza; gh indisponível não admite confirmação manual como substituto.
- [x] Depois de squash, remover somente caminhos concluídos cujo blob ainda coincide; preservar propostas novas e modificadas.
- [x] A branch proposals recebe só a limpeza necessária, sem lock de promoção ou merge obrigatório de main.
- [x] Leitura local acompanha main após a conclusão; nenhuma limpeza automática apaga worktrees de edição.
- [x] Testes inspecionam arquivos remotos, commit de limpeza e leitura; registrar contagem real.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: integration; internal/consolidation/finish_test.go com pelo menos 4 testes de PR aberto, squash merge com propostas novas/alteradas, atualização da leitura e falha de consulta.
**Gate**: Full — go test ./...
**Commit**: `feat(consolidation): finish merged publications`

**Result**: Complete — 4 testes novos de conclusão e gate completo passaram; squash, nova proposta, proposta alterada, PR aberto e falha de confirmação cobertos.

### T7: Trocar a interface MCP de promoção

**What**: Substituir o contrato MCP antigo pelos quatro handlers pequenos, migrando no mesmo corte seus testes de transporte e E2E.
**Where**: `internal/mcpserver/server.go`
**Depends on**: T6
**Reuses**: envelopes MCP atuais, núcleo consolidation, inbox e fixtures offline
**Requirement**: PROM-01, PROM-03, PROM-08, PROM-22, PROM-23, PROM-25, PROM-26, PROM-27, PROM-28, PROM-29, PROM-30, PROM-32

**Done when**:

- [x] Expose prepare/check/submit/finish; remova open/status, previews/applies/batch, caches e startup recovery.
- [x] Preparar conclui primeiro a publicação anterior mergeada; PR aberto é informado sem iniciar outra promoção.
- [x] A listagem de propostas do inbox e a leitura publicada não dependem dos pacotes antigos; source release deixa de ser aceito.
- [x] Os testes de transporte que chamavam endpoints removidos são substituídos por testes dos arquivos atuais, junto da mudança.
- [x] TestManagedWorkflowEndToEnd passa por preparação, escrita direta, revisão/edição, PR, squash e limpeza usando serviços simulados.
- [x] Preservar os testes existentes de captura, setup, busca, expansão e leitura; registrar contagens e substituições explícitas.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: integration/e2e; migrar testes de runtime em internal/mcpserver, tests/claudeadapter/remember_inbox_test.go e tests/e2e/phase3_test.go; incluir pelo menos 4 testes novos nomeados para contrato, relações many-to-many, preservação de captura/leitura e percurso completo.
**Gate**: Full — go test ./...
**Commit**: `refactor(mcp): expose file-based promotion tools`

**Result**: Complete — 21 testes nomeados de MCP, runtime Claude e E2E com 17 propostas passaram; make verify verde (testes, race, vet, build). Inventário inclui Markdown 100755. Testes antigos substituídos conforme inventário em context.md.

### T8: Manter somente a conclusão explícita na CLI

**What**: Conectar gpm promote --finish ao núcleo novo e retirar resume, abort e fallback manual de merge.
**Where**: `internal/app/app.go`
**Depends on**: T7
**Reuses**: app.Run, config.Load e consolidation.Finish
**Requirement**: PROM-25, PROM-26, PROM-27, PROM-28, PROM-32

**Done when**:

- [x] A ajuda e o parser expõem a conclusão explícita, sem comandos de retomada/aborto ou CLI completa paralela.
- [x] O comando usa confirmação real do GitHub e reporta seus resultados/erros.
- [x] Substituir somente testes das flags/comportamentos removidos; preservar setup, retrieve, MCP e help/version existentes.
- [x] Os testes de CLI e a suíte completa passam; registrar contagem real.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: integration; atualizar internal/app/app_test.go com pelo menos 3 testes para conclusão, merge ainda aberto e rejeição das flags removidas.
**Gate**: Full — go test ./...
**Commit**: `refactor(cli): finish publications without release state`

**Result**: Complete — 7 testes nomeados de promoção na CLI e gate completo passaram; sem resume/abort/yes e sem fallback manual de merge; regressões de setup, captura/leitura e URL preservadas.

### T9: Reescrever o skill para consolidação pelo agente

**What**: Definir o percurso com leitura integral, plano na conversa, edição direta, delegação por memória e revisão humana.
**Where**: `integrations/claude-code/skills/promote/SKILL.md`
**Depends on**: T8
**Reuses**: quatro ferramentas MCP novas e ferramentas nativas verificadas na documentação oficial
**Requirement**: PROM-03, PROM-06, PROM-07, PROM-09, PROM-10, PROM-11, PROM-12, PROM-13, PROM-14, PROM-17, PROM-18, PROM-20, PROM-21, PROM-33

**Done when**:

- [x] O skill orienta leitura de todos os insumos e memórias, apresenta agrupamentos/destinos/justificativas e espera aprovação antes da escrita.
- [x] Delegação atribui um responsável por memória e fornece contexto; principal revisa coerência; nenhuma rodada escreve durante edição humana.
- [x] Ajustes leem arquivos atuais; mudanças materiais voltam à conversa; publicação segue aprovação do resultado atual.
- [x] Não há plano persistente, lote de dezesseis, ledger por fato, folds, códigos fechados ou tokens.
- [x] allowed-tools inclui ferramentas nativas e as quatro MCP; documentar que permissões do host continuam valendo.
- [x] Os testes provam obrigações escritas, sem alegar executar julgamento LLM; o Verifier julga exemplos e limites.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: contract; substituir tests/claudeadapter/promote_create_test.go por testes nomeados do novo contrato, pelo menos 15 testes, mais testes de instalação existentes.
**Gate**: Full — go test ./...
**Commit**: `refactor(promote): guide agent-led consolidation`

**Result**: Complete — 15 testes nomeados do contrato do skill e gate completo passaram; skill orienta arquivos finais e dois momentos de aprovação, sem operações por proposta ou contrato antigo.

### T10: Retirar o componente antigo de promoção e release

**What**: Remover exclusivamente internal/promotion e internal/release, seus testes específicos e os helpers de release que ficaram órfãos.
**Where**: `internal/`
**Depends on**: T9
**Reuses**: regressões do fluxo novo e componentes compartilhados preservados
**Requirement**: PROM-28, PROM-29, PROM-30

**Done when**:

- [x] Os dois pacotes antigos deixam de existir; a fixture workspacetest remove OpenRelease e o import antigo.
- [x] Esse é um único corte do componente legado porque os testes de release importam promotion e testes de promotion usam OpenRelease; não deixar commit que não compila.
- [x] Retirar apenas helpers, constantes e paths que ficaram sem consumidores; preservar locks/ops usados por leitura, inbox e setup.
- [x] Aprovação desta tarefa autoriza retirar testes exclusivos de contratos excluídos, com inventário no commit; nunca retirar teste de comportamento preservado para obter gate verde.
- [x] A suíte completa, vet e build passam; comparar contagens e explicar somente remoções previstas.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: regression/integration; go test ./... após a retirada; sem testes artificiais de ausência de arquivos. Os comportamentos suportados já têm testes nos componentes entregues.
**Gate**: Full — go test ./...
**Commit**: `refactor(promotion): remove legacy release engine`

**Result**: Complete — Motor legado e 27 testes exclusivos retirados conforme autorização; regressões suportadas preservadas. make verify (testes, race, vet, build), gate final go test ./... e git diff --check passaram.

### T11: Atualizar o guia do fluxo de uso

**What**: Descrever branches, arquivos finais, revisão e limpeza após merge conforme o fluxo implementado.
**Where**: `docs/workflow.md`
**Depends on**: T10
**Reuses**: spec e design aprovados, comandos e testes novos
**Requirement**: PROM-31

**Done when**:

- [x] O guia explica o caminho feliz, insumos separados, aprovações e limpeza preservando propostas novas.
- [x] Remover instruções de marcador, folds, batches, recovery e retomada; não prometer cenários excluídos.
- [x] Um teste de contrato do documento passa junto da suíte completa.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: contract; criar tests/claudeadapter/promotion_docs_test.go com TestPromotionDocumentationDescribesImplementedWorkflow cobrindo inicialmente este guia.
**Gate**: Full — go test ./...
**Commit**: `docs(workflow): describe direct file consolidation`

**Result**: Complete — Guia de uso alinhado ao percurso aprovado; teste de contrato do guia e go test ./... passaram; links locais e diff conferidos.

### T12: Atualizar a referência de funcionalidades

**What**: Documentar as quatro ferramentas MCP e a CLI de conclusão, removendo contratos e avaliação de fontes obsoletos.
**Where**: `docs/features.md`
**Depends on**: T11
**Reuses**: contrato MCP efetivo e guia atualizado
**Requirement**: PROM-31

**Done when**:

- [x] A referência lista somente as ferramentas/comandos implementados e mantém limites/exclusões corretos.
- [x] A avaliação estruturada de fontes já removida deixa de aparecer como capacidade implementada.
- [x] O teste de documentação mantém cobertura anterior e acrescenta este documento; suíte completa passa.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: contract; ampliar TestPromotionDocumentationDescribesImplementedWorkflow para cobrir também docs/features.md, sem enfraquecer as asserções do guia.
**Gate**: Full — go test ./...
**Commit**: `docs(features): document simplified promotion tools`

**Result**: Complete — Referência atualizada com nove ferramentas MCP, quatro de promoção, parâmetros e limites efetivos; dois documentos cobertos pelo contrato; go test ./... passou.

### T13: Atualizar o uso da integração Claude Code

**What**: Explicar o novo /promote, edição direta e acesso ao caminho externo do worktree conforme permissões da sessão.
**Where**: `integrations/claude-code/README.md`
**Depends on**: T12
**Reuses**: skill e referência oficial de ferramentas/permissões
**Requirement**: PROM-31

**Done when**:

- [x] O guia orienta revisão dos mesmos arquivos pelo terminal/editor, sem abertura automática ou estado retomável.
- [x] Acesso ao diretório segue permissões normais do host; não prometer que allowed-tools limita ferramentas ou concede permissão permanente.
- [x] Os testes mantêm asserções anteriores e acrescentam este documento; suíte completa passa.
- [x] Gate da tarefa passa: `go test ./...`.

**Tests**: contract; ampliar TestPromotionDocumentationDescribesImplementedWorkflow para o guia da integração; manter testes de setup/instalação.
**Gate**: Full — go test ./...
**Commit**: `docs(claude): explain file-based promotion review`

**Result**: Complete — Guia Claude alinhado à edição dos arquivos atuais e permissões do host; três documentos cobertos; links locais e oficiais conferidos, go test ./... passou.

### T14: Alinhar o glossário ao modelo simplificado

**What**: Substituir termos de transação, folds, marcador e recovery pelos conceitos de consolidação, worktree e publicação.
**Where**: `CONTEXT.md`
**Depends on**: T13
**Reuses**: vocabulário do novo fluxo
**Requirement**: PROM-31

**Done when**:

- [x] O glossário deixa de estabelecer como modelo atual operações removidas e diferencia comprovante de publicação de sessão retomável.
- [x] O teste de documentação cobre os quatro arquivos; não expandir o restante do README ou docs de pesquisa.
- [x] make verify passa com todos os testes, race, vet e build; registrar os totais finais.
- [x] Gate da tarefa passa: `make verify`.

**Tests**: contract; completar TestPromotionDocumentationDescribesImplementedWorkflow com o glossário, mantendo os três documentos anteriores.
**Gate**: Build — make verify
**Commit**: `docs(context): align glossary with consolidation flow`

**Result**: Complete — Glossário e quatro documentos cobertos pelo contrato; 173 testes nomeados de topo disponíveis. make verify passou: suíte completa, race, vet e build. Prova C32 ampliada para incluir falhas já testadas na preparação, envio e conclusão.

## Checks

Os comandos abaixo são provas planejadas: os testes serão criados nas tarefas correspondentes. Cada seletor identifica um teste nomeado. As provas de skill verificam o contrato escrito; o walk e o Verifier avaliam exemplos semânticos, sem confundir essa avaliação com execução de LLM automatizada. A retirada física do motor antigo também será auditada no diff e no gate de build.

**C1** - O worktree parte da main obtida no fetch, sem histórico de proposals (PROM-01, Preparar e compreender, AC 1).
Proof: `go test ./internal/consolidation -run '^TestPrepareStartsFromFetchedMain$' -count=1`

**C2** - Todas as propostas da revisão fixa têm cópias com bytes integrais (PROM-02, Preparar e compreender, AC 2).
Proof: `go test ./internal/consolidation -run '^TestPrepareCopiesEveryProposalFromPinnedRevision$' -count=1`

**C3** - O contrato do skill exige leitura integral de todas as memórias antes do plano (PROM-03, Preparar e compreender, AC 3).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractRequiresFullMemoryRead$' -count=1`

**C4** - Uma preparação inclui 17 propostas no mesmo conjunto, sem lote de 16 (PROM-04, Preparar e compreender, AC 4).
Proof: `go test ./internal/consolidation -run '^TestPrepareIncludesSeventeenProposals$' -count=1`

**C5** - Uma proposta posterior não entra nos insumos já preparados (PROM-05, Preparar e compreender, AC 5).
Proof: `go test ./internal/consolidation -run '^TestPreparedInputsExcludeLaterProposals$' -count=1`

**C6** - O contrato exige grupos com propostas, destinos e mudanças na conversa (PROM-06, Discutir o plano, AC 1).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractPresentsGlobalGroups$' -count=1`

**C7** - O contrato exige destino ou justificativa livre para cada proposta inicial (PROM-07, Discutir o plano, AC 2).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractAccountsForEveryProposal$' -count=1`

**C8** - O fluxo aceita 3 propostas alimentando 2 memórias, inclusive uma proposta usada nas duas (PROM-08, Discutir o plano, AC 3).
Proof: `go test ./internal/mcpserver -run '^TestConsolidationSupportsManyToManyMapping$' -count=1`

**C9** - O contrato impede escrita canônica antes da aprovação do plano (PROM-09, Discutir o plano, AC 4).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractWaitsForPlanApproval$' -count=1`

**C10** - O contrato permite revisar grupos, destinos e contradições na conversa (PROM-10, Discutir o plano, AC 5).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractRevisesPlanInConversation$' -count=1`

**C11** - O contrato limita reorganização às memórias relacionadas, preservando conhecimento válido (PROM-11, Discutir o plano, AC 6).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractLimitsReorganization$' -count=1`

**C12** - O contrato exige memória canônica própria em vez de copiar instruções da proposta (PROM-12, Preparar e revisar, AC 1).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractWritesCanonicalKnowledge$' -count=1`

**C13** - O contrato delega por arquivo com responsável exclusivo, insumos e contexto aprovado (PROM-13, Preparar e revisar, AC 2).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractDelegatesByMemory$' -count=1`

**C14** - O contrato exige revisão global pelo principal antes da revisão humana (PROM-14, Preparar e revisar, AC 3).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractReviewsWholeResult$' -count=1`

**C15** - IDs mantidos não mudam e novos headings recebem IDs canônicos antes da revisão (PROM-15, Preparar e revisar, AC 4).
Proof: `go test ./internal/memory -run '^TestPrepareDocumentPreservesAndCompletesIDs$' -count=1`

**C16** - A conferência rejeita categorias ou documentos incompatíveis com a leitura (PROM-16, Preparar e revisar, AC 5).
Proof: `go test ./internal/consolidation -run '^TestCheckAcceptsOnlyCanonicalDocuments$' -count=1`

**C17** - O contrato apresenta resumo, diff atual por memória e caminho de edição (PROM-17, Preparar e revisar, AC 6).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractReviewsCurrentFilesAndDiff$' -count=1`

**C18** - O contrato suspende escrita dos agentes durante revisão/edição humana (PROM-18, Preparar e revisar, AC 7).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractPausesForHumanEditing$' -count=1`

**C19** - O commit conserva uma edição humana feita nos arquivos atuais após a primeira escrita (PROM-19, Preparar e revisar, AC 8).
Proof: `go test ./internal/consolidation -run '^TestSubmitPreservesHumanEdits$' -count=1`

**C20** - O contrato retorna à pessoa quando surge mudança material de conteúdo no plano (PROM-20, Preparar e revisar, AC 9).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractDiscussesMaterialPlanChanges$' -count=1`

**C21** - O contrato exige aprovação do resultado atual antes do submit, sem preview token (PROM-21, Preparar e revisar, AC 10).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractRequiresApprovalOfCurrentResult$' -count=1`

**C22** - Um único commit contém só memórias aprovadas, sem propostas ou auxiliares (PROM-22, Publicar e concluir, AC 1).
Proof: `go test ./internal/consolidation -run '^TestSubmitCommitsOnlyCanonicalChanges$' -count=1`

**C23** - O envio publica a branch, abre PR para main e retorna seu link sem merge automático (PROM-23, Publicar e concluir, AC 2).
Proof: `go test ./internal/consolidation -run '^TestSubmitPushesBranchAndCreatesMainPR$' -count=1`

**C24** - O PR recebe resumo livre com destinos e justificativas das propostas concluídas (PROM-24, Publicar e concluir, AC 3).
Proof: `go test ./internal/consolidation -run '^TestSubmitRecordsFreeFormProposalSummary$' -count=1`

**C25** - Propostas permanecem no remoto enquanto o merge não está confirmado (PROM-25, Publicar e concluir, AC 4).
Proof: `go test ./internal/consolidation -run '^TestFinishKeepsProposalsUntilConfirmedMerge$' -count=1`

**C26** - Após squash, só caminhos/blobs concluídos iguais são removidos; novos e alterados permanecem (PROM-26, Publicar e concluir, AC 5).
Proof: `go test ./internal/consolidation -run '^TestFinishAfterSquashPreservesNewAndChangedProposals$' -count=1`

**C27** - A leitura local passa a refletir main após a limpeza (PROM-27, Publicar e concluir, AC 6).
Proof: `go test ./internal/consolidation -run '^TestFinishRefreshesPublishedReads$' -count=1`

**C28** - O contrato MCP tem quatro ferramentas de arquivos e retira operações/tokens/folds antigos (PROM-28, Substituir contrato, AC 1).
Proof: `go test ./internal/mcpserver -run '^TestPromotionToolContractUsesFiles$' -count=1`

**C29** - Preparar e enviar não publicam nem interpretam release.lock.json (PROM-29, Substituir contrato, AC 2).
Proof: `go test ./internal/consolidation -run '^TestPrepareAndSubmitDoNotUseSharedReleaseMarker$' -count=1`

**C30** - Remember continua publicando propostas e leituras continuam usando apenas main publicada (PROM-30, Substituir contrato, AC 3).
Proof: `go test ./internal/mcpserver -run '^TestConsolidationPreservesRememberAndPublishedReads$' -count=1`

**C31** - Os quatro documentos descrevem o fluxo implementado e retiram contratos removidos (PROM-31, Substituir contrato, AC 4).
Proof: `go test ./tests/claudeadapter -run '^TestPromotionDocumentationDescribesImplementedWorkflow$' -count=1`

**C32** - Falha de dependência interrompe com erro e deixa o trabalho preparado, sem alegar conclusão (PROM-32, Edge Cases, AC 1).
Proof: `go test ./internal/consolidation -run '^(TestConsolidationFailureStopsWithoutDeletingWork|TestSubmitFailureKeepsPreparedWork|TestFinishFailurePreservesProposalsAndWork)$' -count=1`

**C33** - O contrato explicita contexto insuficiente e proíbe apresentar leitura parcial como completa (PROM-33, Edge Cases, AC 2).
Proof: `go test ./tests/claudeadapter -run '^TestPromoteContractStopsOnIncompleteContext$' -count=1`

**C34** - Sem propostas, preparar informa a ausência e não cria publicação (PROM-34, Edge Cases, AC 3).
Proof: `go test ./internal/consolidation -run '^TestPrepareWithoutProposalsDoesNotPublish$' -count=1`

## Traceability

| Requirements | Tasks |
| --- | --- |
| PROM-01, PROM-02, PROM-04, PROM-05, PROM-34 | T2, T3, T7 |
| PROM-03, PROM-06, PROM-07, PROM-09, PROM-10, PROM-11 | T9 |
| PROM-08 | T5, T7 |
| PROM-12, PROM-13, PROM-14 | T9 |
| PROM-15, PROM-16 | T1, T4 |
| PROM-17, PROM-18, PROM-20, PROM-21 | T4, T9 |
| PROM-19 | T4, T5, T9 |
| PROM-22, PROM-23, PROM-24 | T5, T7 |
| PROM-25, PROM-26, PROM-27 | T6, T7, T8 |
| PROM-28, PROM-29, PROM-30 | T3, T5, T7, T8, T10 |
| PROM-31 | T11, T12, T13, T14 |
| PROM-32 | T3, T4, T5, T6, T7, T8 |
| PROM-33 | T9 |

**Coverage**: 34 critérios, 34 checks e 34 provas implementadas; validação final pendente.

## Task Granularity Check

T1–T9 entregam um componente ou interface coesa com seus próprios testes. T10 é a remoção atômica do motor legado acoplado: os pacotes release/promotion têm dependência cruzada em testes e fixtures, e sua retirada separada quebraria o build. T11–T14 alteram um documento por tarefa, acrescentando seu contrato de verificação junto da mudança.

As dependências dos diagramas correspondem aos campos Depends on; fases apontam apenas para tarefas anteriores. Os gates e tipos de teste correspondem à matriz. A migração dos testes MCP/E2E acompanha T7, não uma tarefa posterior.

## Handoff

**Settled mid-build:** T1–T14 concluídas com gates verdes; provas finais e Verifier pendentes. Execução sequencial pelo principal, aprovada com "aprovado".
**Dispatched:** None.
**Current:** T14 concluída com gate verde; executar provas em HEAD e despachar Verifier independente.
**Environment:** `source .local/promote-env.sh`; Go 1.25.12 e Zig 0.14.1 isolados em `/tmp/promote-tools`, ambos com checksums oficiais conferidos. CGO habilitado para race. Wrapper local de Go acrescenta `-count=1` aos testes para evitar travamento observado no cache de inputs de leitores `/proc/self/fd`; build cache permanece ativo. Nenhuma configuração global alterada.
**Walked:** 2026-11-03 — Independent Tests percorridos em Git local e MCP/CLI reais, sem serviços remotos: preparação de 17 propostas e inventário fixo (consolidation/prepare_test.go); relações 3 propostas/2 páginas e leitura preservada (mcpserver/consolidation_test.go); páginas finais, IDs e edição humana (memory/document_test.go, consolidation/check_test.go e submit_test.go); PR aberto sem limpeza, squash humano simulado, preservação de propostas novas/alteradas e leitura atualizada (consolidation/finish_test.go); substituição do contrato antigo (MCP, CLI e quatro documentos). TestManagedWorkflowEndToEnd compila gpm, executa setup, captura, edição direta, publicação, merge simulado e conclusão em processo novo; segunda pessoa lê sem publicar. Os quatro cenários stdio exercitam criação, atualização, reorganização e combinação. Os testes da skill verificam política escrita; não executam LLM nem provam julgamento semântico automatizado, reservado ao Verifier. Sem telas/capturas nesta feature.

Há 14 tarefas em quatro fases. Podem ser executadas pelo principal ou, com autorização explícita, por dois trabalhadores sequenciais: fases 1+2 (7 tarefas), depois fases 3+4 (7 tarefas). A delegação desta implementação é distinta da delegação por memória prevista no produto. Nenhum trabalhador foi despachado.

Após a última tarefa: check_proofs em HEAD, Verifier independente com três mutações, fechamento dos achados e validate_verification. Não há capturas ou telas para despachar screens. Verifier é obrigatório pela skill, independentemente da escolha sobre trabalhadores de implementação.

