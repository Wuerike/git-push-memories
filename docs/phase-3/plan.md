# Plano da Fase 3: curadoria e confianca

Status do plano: **T1-T9 implementadas e verificadas; T10 pendente**.

## Objetivo

Transformar propostas Markdown do inbox em memoria canonica atual, coerente e recuperavel, com revisao explicita, validacoes deterministicas e historico Git auditavel.

O fluxo alvo e:

```text
proposta commitada no inbox
-> selecao e leitura
-> recuperacao de memoria relacionada
-> escolha da operacao canonica
-> preview do diff completo
-> confirmacao humana
-> revalidacao e apply
-> commit de memoria + consumo ou descarte da proposta
-> recuperacao em outro processo
```

A fase possui tres incrementos. A Fase 3A fecha o primeiro ciclo para uma proposta `create`. A Fase 3B completa operacoes e garantias de confianca. A Fase 3C mede valor e custo de curadoria antes de autorizar captura automatica.

## Fontes de decisao

- O [roadmap](../roadmap.md#fase-3-curadoria-e-confianca) define resultado, fronteiras, incrementos e gate.
- A [ADR 0001](../adr/0001-implementation-base.md) fixa Go, Markdown e Git como autoridade duravel, escrita byte-preserving, drift e publicacao isolada.
- A [ADR 0004](../adr/0004-markdown-proposal-inbox-and-memory-categories.md) fixa o inbox Markdown em `main`, categorias configuradas e consumo de propostas no mesmo commit da alteracao canonica.
- A [comparacao dos mecanismos de geracao](../memory-generation-mechanisms-comparison.md) identifica consolidacao, grounding, deduplicacao e consistencia temporal como as principais lacunas do fluxo atual.
- O [estado das funcionalidades](../functionality-status.md) e a autoridade operacional sobre o que ja esta disponivel.

Se este plano divergir de uma ADR aceita, a ADR prevalece. Mudancas na autoridade duravel, identidade de secao, fronteira de privacidade ou lifecycle Git exigem nova decisao arquitetural.

## Decisoes operacionais

| Tema | Decisao da Fase 3 |
|---|---|
| interface humana | skill explicita `/curate`; a pessoa seleciona a proposta e confirma o diff exato |
| transporte | MCP transitorio sobre stdio; preview single-use mantido apenas no processo que o emitiu |
| autoridade | o agente classifica e recomenda; somente o GPM valida e escreve; a pessoa autoriza o apply |
| proposta | Markdown commitado em `proposals/`; acao e target sao sugestoes, nao operacoes autoritativas |
| memoria | somente `memories/<categoria-configurada>/**/*.md` e canonico e recuperavel |
| consolidacao | memoria relacionada deve ser considerada novamente no preview, porque pode ter mudado desde a captura |
| privacidade | schema e todos os campos da proposta sao revalidados antes de listagem detalhada, preview e apply |
| preview | nao escreve; vincula proposta, `HEAD`, target e post-state; qualquer ajuste exige novo preview |
| apply | revalida configuracao, proposta, privacidade, `HEAD`, target, hashes e post-state |
| Git | commit construido a partir de `HEAD` com indice isolado e compare-and-swap de `main`; staging alheio nao entra |
| consumo | o mesmo commit adiciona ou altera memoria e remove as propostas consumidas; descarte tambem e observavel em Git |
| remotos | o produto nao executa fetch, pull, push, merge nem cria pull request |
| sessoes | prompts, respostas e transcripts nao entram no fluxo nem em estado temporario |

## Interfaces externas

Skills:

```text
/remember <fato>
/curate [proposal-id]
```

Tools MCP de curadoria:

```text
memory_proposal_list
memory_curate_preview
memory_curate_apply
memory_curate_batch_preview
memory_curate_batch_apply
```

`memory_curate_preview` exige uma operacao explicita. Para uma proposta `create`, `create_page` recebe um path novo em categoria configurada e `create_section` recebe uma pagina existente e um heading novo. Para uma proposta `update`, `replace_section` recebe pagina e section ID de uma secao folha. O retorno inclui proposta, fontes, memoria relacionada, operacao, target, section ID, diff e token single-use. `memory_curate_apply` aceita somente esse token e o invalida na primeira tentativa, inclusive quando ocorre drift.

Os nomes podem ser estendidos sem criar um caminho alternativo que aplique conteudo sem preview e confirmacao. O servidor nao consegue provar por si so que a confirmacao foi humana; essa sequencia pertence ao contrato testado da skill, como ja ocorre em `/remember`.

## Contratos dos modulos

```text
Inbox
list() -> ProposalSnapshot[]
read(proposalId) -> ProposalSnapshot
confirm(snapshot) -> Current | Drift

Curation
list() -> Categories + ProposalSummary[]
preview(operation, proposalId, targetPath, sectionId?, contentHash?, heading?, replacement?) -> CurationPreview
apply(preview) -> CurationResult | Drift

Memory Repository
previewCreatePage(request) -> CanonicalPostState + Diff
previewCreateSection(request) -> CanonicalPostState + Diff
previewReplaceSection(request) -> CanonicalPostState + Diff
reconcileCreatePage(expected) -> Baseline | PostState | Drift
reconcileCreateSection(expected) -> Baseline | PostState | Drift
reconcileReplaceSection(expected) -> Baseline | PostState | Drift
rollback(expectedPostState, baseline?) -> Restored | Drift

Retrieval
retrieve(proposalKnowledge, budget) -> RelatedMemory

Claude Adapter
curateExplicit(proposalId?) -> List + Preview + Confirmation + ApplyResult
```

Regras de dependencia:

- Inbox conhece o schema e a identidade das propostas, mas nao decide operacoes canonicas.
- Curation coordena Inbox, Intake Guard, Memory Repository, Retrieval e Git.
- Memory Repository continua sendo a autoridade sobre schema e renderizacao do Markdown canonico.
- Retrieval permanece read-only e nao considera propostas como memoria autoritativa.
- Claude Adapter nao conhece parser, filesystem, indices Git ou implementacao dos locks.
- Captura e curadoria compartilham o mesmo lock por repositorio para serializar atualizacoes de `main` no mesmo clone.

## Ordem de execucao

```text
T1 create page por /curate
-> T2 create section
-> T3 replace section
-> T4 remove section e descartar proposta
T2 + T3 + T4 -> T5 duplicidade e contradicao observaveis
T3 + T5 -> T6 referencias verificaveis
T2 + T3 + T4 + T5 + T6 -> T7 transacao multi-path e recovery
T7 -> T8 E2E tecnico da Fase 3B
T8 -> T9 protocolo de valor e custo de curadoria
T9 -> T10 experimento, relatorio e decisao do gate
```

Cada tarefa entrega testes e atualiza o inventario operacional. O E2E e o experimento nao devem ser usados para acumular verificacoes que pertencem aos modulos implementados antes deles.

## Tarefas

| Tarefa | Incremento | Estado | Resultado |
|---|---|---|---|
| [T1](t1-create-page-curation.md) | 3A | implementada | uma proposta `create` vira pagina canonica e e consumida no mesmo commit |
| [T2](t2-create-section-curation.md) | 3B | implementada | `create` pode acrescentar uma secao a uma pagina existente |
| [T3](t3-replace-section-curation.md) | 3B | implementada | `update` substitui uma secao folha identificada sem acumular estado antigo |
| [T4](t4-remove-and-discard-curation.md) | 3B | implementada | `remove` e descarte possuem preview, confirmacao e decisao Git observavel |
| [T5](t5-duplicate-and-contradiction-evidence.md) | 3B | implementada | duplicidades e possiveis contradicoes aparecem no preview e exigem decisao |
| [T6](t6-verifiable-source-references.md) | 3B | implementada | referencias identificam sua origem e sua integridade e verificada quando disponivel localmente |
| [T7](t7-multi-path-transaction-recovery.md) | 3B | implementada | lotes e falhas nas janelas de publicacao sao reconciliados sem commit duplicado ou lock indevido |
| [T8](t8-technical-e2e.md) | 3B | implementada | E2E reproduz todas as operacoes e garantias do gate tecnico vigente |
| [T9](t9-value-evaluation-protocol.md) | 3C | implementada | protocolo pre-registra corpus, metricas, limiares e decisoes possiveis sem executar inferencia autenticada |
| T10 | 3C | pendente | experimento produz relatorio e decisao entre avancar, revisar ou interromper |

T2-T7 devem receber documentos proprios quando seu tracer bullet iniciar. T8-T10 devem criar, respectivamente, protocolo e relatorio tecnico ou de valor; resultados nao devem ser escritos antecipadamente.

## T2: criar secao em pagina existente

**Cenario ou hipotese:** uma proposta `create` relacionada a uma pagina existente pode acrescentar uma secao sem criar pagina duplicada nem alterar secoes preexistentes.

**Escopo:** selecionar pagina existente, gerar heading e section ID estavel, apresentar diff byte-preserving, aplicar e consumir a proposta no mesmo commit.

**Fora de escopo:** atualizar secao existente, merge semantico entre propostas e lotes.

**Criterio de conclusao:** a nova secao e recuperavel; bytes fora da insercao permanecem iguais; colisao de heading ou ID falha sem escrita; proposta e staging alheio sao preservados em qualquer falha anterior ao commit.

**Dependencia e decisao habilitada:** depende de T1 e confirma a segunda operacao de consolidacao de `create`.

## T3: substituir secao existente

**Cenario ou hipotese:** uma proposta `update` pode substituir o corpo de uma secao folha e manter somente o estado atual como autoritativo.

**Escopo:** target com path, section ID e content hash observado; construcao de corpo substituto completo que preserve fatos ainda validos; preview byte-preserving; apply e consumo no mesmo commit.

**Fora de escopo:** editar headings ou anchors, atualizar secoes nao folha e merge automatico de contradicoes.

**Criterio de conclusao:** a recuperacao retorna somente o novo estado; identidade e estrutura da pagina nao mudam; target ambiguo, ausente ou obsoleto falha sem escrita.

**Dependencia e decisao habilitada:** depende de T2 apenas para reutilizar o fluxo de escolha de target; libera remocao e tratamento de contradicoes sobre identidade precisa.

## T4: remover conhecimento e descartar proposta

**Cenario ou hipotese:** remocao canonica e rejeicao de uma proposta podem ser revisadas e auditadas sem manter um store paralelo de receipts.

**Escopo:** remover uma secao folha ou pagina elegivel; descartar proposta sem mudar memoria; reason codes fechados; commit com os paths exatos.

**Fora de escopo:** cascade delete, reparo automatico de links e politicas remotas de aprovacao.

**Criterio de conclusao:** a memoria removida deixa de ser recuperada; descarte nao altera memoria; Git distingue consumo e descarte; operacao invalida ou cancelada nao remove a proposta.

**Dependencia e decisao habilitada:** depende de T3 para identidade e drift; completa os destinos individuais do inbox.

## T5: tratar duplicidade e contradicao

**Cenario ou hipotese:** busca deterministica e analise assistida sobre candidatos limitados tornam equivalencias e conflitos observaveis sem delegar a decisao ao modelo.

**Escopo:** dedupe exato no inbox; recuperacao separada de candidatos canonicos e propostas relacionadas; classificacao assistida; apresentacao da evidencia no preview; decisao humana.

**Fora de escopo:** embeddings, banco vetorial, resolucao autonoma e garantia de completude semantica.

**Criterio de conclusao:** proposta identica nao gera publicacao duplicada; candidatos relacionados sao identificaveis; contradicao sugerida nunca escreve sem escolha explicita; falhas conhecidas da busca ficam registradas para a Fase 5.

**Dependencia e decisao habilitada:** depende das operacoes T2-T4; confirma se busca textual e contexto limitado sao suficientes para a curadoria inicial.

## T6: verificar referencias

**Cenario ou hipotese:** referencias estruturadas podem provar origem e integridade sem persistir trechos privados ou exigir acesso remoto.

**Escopo:** definir identidade nao ambigua de repositorio, revisao, path e hash; validar localmente existencia e conteudo; distinguir integridade deterministica de sustentacao semantica.

**Fora de escopo:** provar entailment automaticamente, buscar remotos, executar codigo arbitrario e armazenar transcripts.

**Criterio de conclusao:** referencia verificada e reproduzivel no checkout indicado; fonte ausente ou divergente nao aparece como verificada; a pessoa consegue revisar qual evidencia sustenta a mudanca.

**Dependencia e decisao habilitada:** depende de target preciso e da classificacao de T5; decide o schema de evidencia necessario para o gate.

## T7: transacao multi-path e recovery

**Cenario ou hipotese:** uma transacao Git preparada e identificavel pode consumir varias propostas e reconciliar interrupcoes sem publicar commit parcial, duplicar commit ou remover lock alheio.

**Escopo:** lotes limitados, hashes de todos os inputs e outputs, commit unico, marcador privado sem conteudo, fault injection e recovery nas janelas antes e depois de `update-ref`.

**Fora de escopo:** transacao entre clones, push, merge e banco de coordenacao.

**Criterio de conclusao:** falhas injetadas em cada ponto duravel convergem para baseline ou commit completo; retries reconhecem o commit existente; index e worktree sao reconciliados; lock estrangeiro nunca e removido.

**Dependencia e decisao habilitada:** depende de todas as operacoes e validacoes individuais; libera o E2E tecnico da Fase 3B.

## T8: executar E2E tecnico da Fase 3B

**Cenario ou hipotese:** os modulos compoem o lifecycle vigente completo em repositorios descartaveis, sem estado de sessao e sem acesso remoto.

**Escopo:** partir de `gpm.json`, gerar propostas sinteticas para todos os destinos, curar, reiniciar processos, recuperar estado final e inspecionar privacidade, Git e artefatos.

**Criterio de conclusao:** create, update, remove e discard produzem o estado esperado; duplicidades e contradicoes sao observaveis; referencias possuem o status correto; reinicio e recovery convergem; a matriz completa de verificacao passa.

**Dependencia e decisao habilitada:** depende de T7; fecha o gate tecnico e libera a pre-definicao do experimento de valor.

## T9: preparar avaliacao de valor

**Cenario ou hipotese:** criterios registrados antes das runs permitem decidir se a memoria consolidada melhora tarefas e se o custo de curadoria e aceitavel.

**Escopo:** corpus, pares com/sem memoria, isolamento, metricas, limiares, tratamento de runs invalidas e decisoes `avancar`, `revisar o fluxo` ou `interromper`.

**Criterio de conclusao:** outro avaliador consegue executar o protocolo sem criar novos criterios ou conhecer as sessoes que originaram as propostas.

**Dependencia e decisao habilitada:** depende de T8; autoriza a execucao classificatoria, nao a Fase 4.

## T10: executar avaliacao e decidir o gate

**Cenario ou hipotese:** o fluxo atual fornece contexto util em uma sessao posterior com custo de manutencao baixo o suficiente para receber captura automatica.

**Escopo:** executar o protocolo T9, preservar evidencias sanitizadas, publicar relatorio e registrar a decisao.

**Criterio de conclusao:** todas as runs possuem validade explicada; resultados sao reproduziveis; metricas de tarefa, recuperacao, curadoria e privacidade estao ligadas aos limiares; existe uma decisao explicita.

**Dependencia e decisao habilitada:** depende de T9. Somente resultado `avancar` autoriza iniciar a Fase 4.

## Verificacao

Cada tarefa de implementacao executa, no minimo:

```text
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

O comando normativo do repositorio e `make verify`. Testes focados devem conduzir os tracer bullets, mas a matriz completa deve passar antes de considerar uma tarefa implementada.

## Gate de conclusao

A Fase 3 termina quando:

- propostas validas podem criar, atualizar ou remover memoria canonica recuperavel;
- propostas incorretas podem ser descartadas sem alterar memoria;
- duplicidades e contradicoes sao tratadas de forma observavel;
- referencias invalidas nao aparecem como verificadas;
- commits e recovery preservam staging alheio e nao deixam propostas consumidas no inbox;
- o fluxo compartilhado e o push humano estao documentados;
- o experimento T10 foi executado sobre o lifecycle vigente;
- a decisao `avancar` foi sustentada pelos criterios pre-registrados antes de iniciar a Fase 4.

Se o experimento nao sustentar `avancar`, a fase permanece aberta com decisao `revisar o fluxo` ou encerra a expansao de escopo.
