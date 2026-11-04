# Promote Simplification Context

**Gathered:** 2026-11-03
**Status:** Especificação, abordagem A e tarefas aprovadas; implementação sequencial pelo principal iniciada.
**Source:** `.local/promote-simplification.md`

## Feature Boundary

Simplificar o `/promote` para analisar todas as propostas pendentes e todas as memórias, discutir um plano global e preparar arquivos finais em um worktree separado. A pessoa revisa os arquivos e o diff antes de aprovar a publicação.

## Implementation Decisions

### Direção registrada no documento de entrada

- Considerar todas as propostas presentes na preparação. Propostas recebidas depois ficam para outra execução.
- Fazer levantamento integral das memórias existentes. Explicitar qualquer limitação de contexto sem declarar uma análise parcial como completa.
- Permitir várias propostas por memória e várias memórias por proposta.
- Apresentar agrupamentos, destinos, mudanças e justificativas para propostas que não se tornarão memória.
- Obter aprovação do plano global antes de preparar as memórias finais.
- Usar arquivos finais e o diff como material de revisão, preservando edições humanas nas rodadas seguintes.
- Retirar o limite de dezesseis, folds, preview tokens e a interface obrigatória de operações por proposta.
- Remover o lock compartilhado e o marcador de release da promoção. Examinar separadamente os locks usados por leitura e checkout.
- A delegação pertence ao skill, por memória final e com um responsável por arquivo. O principal cuida de Git, propostas e publicação.
- Não criar scheduler, filas persistentes, locks por página ou entidades persistentes de grupos.
- Não remover automaticamente formato canônico, categorias ou IDs de seção, pois são consumidos pela leitura.
- A limpeza preserva propostas novas e arquivos auxiliares ficam fora da publicação. A referência da fonte a trabalho recuperável não autoriza implementar retomada: a decisão posterior abaixo exclui essa capacidade.

Esses pontos vêm da seção “Direção acordada” e das regras do documento de entrada. Não constituem aprovação da especificação ou das tarefas desta execução.

### Decisões aprovadas nesta discussão

**Confirmed:** 2026-11-03 - user: "concordo, aprovado"

A resposta aprova as três recomendações apresentadas na rodada anterior. A especificação completa e as tarefas ainda não foram apresentadas para aprovação.

| Tema | Recomendação para discussão | Estado |
| --- | --- | --- |
| Base do trabalho | Criar a branch a partir de `origin/main` e ler as propostas de uma revisão fixa de `origin/proposals`, mantendo insumos fora do commit de memórias. Evita carregar o histórico das propostas para a `main`. | Aprovado |
| Revisão inicial | Usar a conversa no terminal e informar o caminho do worktree para edição opcional no VS Code, sem abrir o editor automaticamente. Ambos revisam os mesmos arquivos. | Aprovado |
| Publicação e limpeza | Manter PR para `main`, com um commit global por consolidação aprovada. Remover as propostas concluídas somente após confirmar o merge, em uma próxima chamada de `/promote` ou comando explícito, preservando propostas novas ou alteradas. | Aprovado |

### Escopo reduzido: sem retomada

**Confirmed:** 2026-11-03 - user: "acho melhor desenvolver focando no cenário mais feliz do fluxo pra não trazer problemas antecipadamente, não quero incluir nada de retomada agora"

- Retomada de uma promoção interrompida está fora desta versão, inclusive reconhecimento de sessão anterior, reconstrução do plano, recuperação automática e continuação automática de trabalho existente.
- A proposta anterior de um trabalho local reconhecido e retomado por `/promote` foi rejeitada. Não implementar metadados para esse fim.
- A orientação é desenvolver o cenário feliz e evitar antecipar mecanismos para exceções. O usuário pediu identificar outros cenários semelhantes e, na resposta seguinte, aprovou as exclusões registradas abaixo.
- A limpeza após merge já aprovada continua no escopo. Registrar o vínculo necessário entre PR e propostas concluídas não deve virar reconstrução da sessão de edição. A forma desse registro ainda precisa ser definida.

### Cenários excluídos pelo usuário

**Confirmed:** 2026-11-03 - user: "aceito remover todos estes pontos"

A resposta aceita retirar os sete cenários apresentados na mensagem anterior: promoções simultâneas; recuperação de falhas; reconciliação automática com avanço da `main`; PR fechado ou promoção abandonada; migração de promoção antiga; divisão automática por volume; caminho especial quando todas as propostas são descartadas. Esses cenários deixam de ser questões abertas desta versão. Retomada já havia sido excluída na resposta anterior.

O escopo fica no percurso preparar → analisar → discutir o plano → escrever → revisar → abrir PR → merge humano → limpar propostas concluídas. Falhas podem ser informadas e interromper o percurso; não implementar recuperação, coordenação ou caminhos alternativos para cobrir esses casos. Essa redução não autoriza apagar trabalho existente nem remover propostas fora do conjunto concluído.

| Cenário | Capacidade que pode ficar para depois | Tratamento simples proposto para esta versão |
| --- | --- | --- |
| Promoções simultâneas | Detecção de sobreposição, coordenação automática e locks remotos | Premissa de uma promoção por vez, coordenada pelo time |
| Falha de rede, Git ou subagente | Retry automático, rollback e recuperação de etapas | Informar o erro e interromper; não apagar trabalho nem declarar sucesso |
| `main` alterada durante a revisão | Rebase ou merge automático e reconciliação do plano | Não resolver conflitos automaticamente; explicar o impedimento quando ocorrer |
| PR fechado ou abandonado | Reabertura automática, substituição de PR e ciclo de abandono | Focar em abrir PR, merge humano e limpeza das propostas concluídas |
| Release antiga pendente | Migração e compatibilidade com dois fluxos completos | Substituir o fluxo; não automatizar a conversão de promoções antigas |
| Volume maior que o contexto | Particionamento automático, paginação inteligente e planejamento em várias sessões | Explicitar a limitação e parar se não for possível analisar o conjunto integral |
| Consolidação composta apenas por descartes | Caminho alternativo de publicação sem mudanças canônicas | Definir depois o suporte a essa exceção; não presumir autorização para apagar propostas |

### Decisões finais do fluxo principal

**Confirmed:** 2026-11-03 - user: "sim"

A resposta aprova as três recomendações da rodada anterior:

1. **Plano na conversa:** apresentar, ajustar e aprovar o plano na própria conversa, sem gerar um arquivo Markdown de plano separado. Registrar no PR o resumo das propostas concluídas e suas justificativas; guardar somente os dados necessários à limpeza após merge, sem salvar sessão para retomada.
2. **Delegação:** o principal decide quando delegar memórias independentes após a aprovação do plano, sem confirmação adicional para cada subagente. O principal revisa o conjunto antes de entregar os arquivos para revisão humana.
3. **Reorganização:** o plano pode propor reorganizar as memórias relacionadas às propostas, preservando o conhecimento ainda válido. Mudanças sem relação com a consolidação ficam fora do escopo; alterações materiais ao plano aprovado voltam à discussão.

Nome/localização do worktree, acesso direto do skill aos arquivos e implementação do registro de limpeza são escolhas técnicas a detalhar no design dentro do comportamento aprovado. A leitura existente exige IDs e formato canônico válidos; a implementação deve preservar essa compatibilidade antes de apresentar o diff final. Isso não autoriza adicionar recuperação, retomada ou uma interface de operações por proposta.

As decisões de produto do fluxo principal foram resolvidas. Os cenários de exceção aceitos para remoção não voltam como requisitos de implementação. Esta confirmação não é aprovação de uma especificação ou de tarefas que ainda não foram apresentadas.

### Aprovação da especificação

**Confirmed:** 2026-11-03 - user: "aprovado"

O usuário aprovou `spec.md` depois de receber seu link e o resumo do escopo. A aprovação foi registrada no cabeçalho da especificação. As tarefas de implementação ainda não foram apresentadas.

### Escolha de arquitetura

**Confirmed:** 2026-11-03 - user: "aceito A"

Manter ferramentas MCP pequenas para preparar, validar, publicar e concluir. O agente lê e edita diretamente os arquivos finais. O núcleo Go cuida do suporte mecânico de Git, formato e GitHub; as decisões sobre o conhecimento ficam com o agente e a pessoa. A opção de CLI como interface principal do skill não será implementada. Permanece o comando explícito de conclusão já aprovado.

### Aprovação das tarefas

**Confirmed:** 2026-11-03 - user: "aprovado"

O usuário aprovou as 14 tarefas apresentadas e a execução sequencial proposta pelo principal. A oferta opcional de trabalhadores não foi solicitada; não despachar trabalhadores de implementação. A aprovação autoriza os commits locais e a substituição explícita dos testes de contratos removidos descrita nas tarefas.

## Repository Findings

- `internal/release/release.go` publica `release.lock.json` em `proposals` e cria o worktree de release a partir da branch de propostas atualizada com a `main`.
- `internal/release/finish.go` participa da finalização após merge.
- `integrations/claude-code/skills/promote/SKILL.md` exige lotes de até dezesseis, decisões por proposta, folds e confirmação de previews.
- `internal/mcpserver/server.go` expõe as ferramentas separadas de preview e aplicação, individual e em lote.
- Não havia `.specs/STATE.md` nem artefatos anteriores desta feature na inspeção inicial. A árvore Git estava limpa em `feat/workflow-branches`.
- A inspeção desta discussão foi estática; testes ainda não foram executados.

## Specific References

- `.local/promote-simplification.md`: fonte do escopo, das decisões já acordadas e das questões abertas.
- `.local/remember-simplification.md`: registra a migração de propostas para Markdown livre e escrita pela API do GitHub.

## Deferred Ideas

- Retomada de promoção interrompida: explicitamente excluída pelo usuário em 2026-11-03.
- Coordenação distribuída e uma infraestrutura própria de agendamento não fazem parte da direção proposta na fonte.
- Os sete cenários da tabela foram excluídos pelo usuário em 2026-11-03, com "aceito remover todos estes pontos".

## Test Contract Migration

T7 substitui os testes dos endpoints de operação por proposta por testes de edição direta, com autorização explícita na aprovação de tasks.md.

Preservados em server_inbox_test.go: `TestMemorySearchAndExpandConfiguredCategory`, `TestMemoryReadFollowsPublishedMain`, `TestReadToolsRejectUnavailableOrUnknownSource`, `TestMemoryProposeCommitsMarkdownToRemoteProposals`, `TestMemoryProposeReportsApiFailuresWithoutWriting`, `TestProposalListRefreshesFromRemote`, `TestProposalListIsReadOnly`, `TestPageReadToolReturnsWholePage`, `TestPageReadToolFailsWhenPageExceedsBudget`, `TestPageReadToolRejectsInvalidRequests`. As quatro rejeições de `source: release` passam a exigir `invalid_source`, pois esse seletor foi retirado.

Contratos antigos substituídos nesse arquivo: `TestPromoteToolsRequireAnOpenRelease`, `TestPromoteCreatePreviewAndApplyPublishesCanonicalMemory`, `TestPromoteCreateSectionUpdatesExistingPageAndConsumesProposal`, `TestPromoteCreateSectionRejectsExistingHeadingWithoutWriting`, `TestPromoteReplaceSectionUpdatesCurrentStateAndConsumesProposal`, `TestPromoteReplaceSectionRejectsNonLeafTargetWithoutWriting`, `TestPromoteReplaceSectionRejectsStaleContentHashWithoutWriting`, `TestPromoteRemoveSectionDeletesLeafAndConsumesProposal`, `TestPromoteRemovePageDeletesEligiblePageAndConsumesProposal`, `TestPromoteRemovePageRejectsIneligiblePageWithoutWriting`, `TestPromoteRemoveSectionRejectsApplyTimeDriftAndConsumesToken`, `TestPromoteDiscardProposalRecordsReasonWithoutChangingMemory`, `TestPromoteDiscardRejectsUnknownReasonAndPreviewCancellationKeepsProposal`, `TestPromotePreviewSeparatesCanonicalAndRelatedProposalEvidence`, `TestPromoteEvidenceCandidatesComeFromTheRelease`, `TestPromoteApplyRejectsDriftInReviewedEvidence`, `TestPromoteApplyRejectsDriftAndConsumesPreviewToken`, `TestPromoteBatchPreviewAndApplyUsesOneCommit`, `TestReleaseOpenStatusConsolidateAndSubmit`, `TestReleaseSourceKeepsHashesAndHandlesConsistentAcrossBatches`, `TestStartupRecoveryFailureOnlyBlocksPromotion`, `TestToolListIncludesPageReadAndReleaseTools`. Seus comportamentos exclusivos de operações, tokens, folds, lock e recovery deixam o produto conforme a spec; captura e leitura permanecem.

Os quatro cenários semânticos do harness stdio (criação, atualização, reorganização de time e duas propostas numa página) serão migrados para arquivos finais e leitura publicada; seus equivalentes não serão descartados.

Os quatro cenários stdio foram migrados para `file_consolidation_test.go` e passaram. Os quatro testes exclusivos de folds/tokens de `rewrite_fold_test.go` foram substituídos pelo contrato MCP sem operações e pelo cenário many-to-many. O teste de runtime da integração Claude foi migrado para preparar arquivos, conferir e enviar PR, mantendo as asserções de isolamento da main e retenção das propostas até merge.

`TestManagedWorkflowEndToEnd` agora cobre setup em remoto vazio, 17 propostas livres com H1, conjunto fixo, proposta recebida depois da captura, edição humana preservada, um commit global canônico, PR sem merge automático, squash humano, limpeza em processo novo e leitura por segunda pessoa sem escrever. Foram preservadas as verificações de ausência de captura automática de fonte privada e de transcrições. Fault injection, preview bindings, retries de envio e restauração de release saíram desse E2E conforme as exclusões aprovadas; a CLI de conclusão será migrada/testada em T8.

O teste da CLI que lista ferramentas MCP mantém a comparação exata, agora com os nove nomes atuais. A integração também fortaleceu `TestConsolidationPreservesRememberAndPublishedReads` com uma memória executável (`100755`), suportada pela leitura existente; o inventário passou a incluir ambos os modos regulares (`100644` e `100755`). Esse ajuste corrige a completude do levantamento sem adicionar um cenário de recuperação. T7 passou em `make verify`, incluindo race, vet e build.
