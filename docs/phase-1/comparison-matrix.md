# Matriz comparativa da Fase 1

Esta matriz e o indice das classificacoes. O significado dos valores e as condicoes de aceitacao estao no [protocolo de avaliacao](evaluation-protocol.md#escala-de-classificacao). Cada valor diferente de `pendente` deve apontar para evidencia no relatorio do candidato ou no registro de licencas da fase.

| Criterio | Basic Memory | ai-memory | Palinode | claude-mem | claude-bedrock | agent-memory | twiceshy |
|---|---|---|---|---|---|---|---|
| Markdown e Git canonicos | [parcial](evaluations/basic-memory.md#criterios) | [parcial](evaluations/ai-memory.md#criterios) | [parcial](evaluations/palinode.md#criterios) | [falha](evaluations/claude-mem.md#criterios) | [parcial](evaluations/claude-bedrock.md#criterios) | [passa](evaluations/agent-memory.md#criterios) | [parcial](evaluations/twiceshy.md#criterios) |
| Operacao local | pendente | pendente | pendente | pendente | [falha](evaluations/claude-bedrock.md#criterios) | pendente | pendente |
| Ausencia de backend obrigatorio | [passa](evaluations/basic-memory.md#criterios) | [falha](evaluations/ai-memory.md#criterios) | [falha](evaluations/palinode.md#criterios) | [falha](evaluations/claude-mem.md#criterios) | [passa](evaluations/claude-bedrock.md#criterios) | [passa](evaluations/agent-memory.md#criterios) | [falha](evaluations/twiceshy.md#criterios) |
| Indice reconstruivel | [passa](evaluations/basic-memory.md#etapa-3-armazenamento-e-recuperacao) | [passa](evaluations/ai-memory.md#etapa-3-armazenamento-e-recuperacao) | pendente | [falha](evaluations/claude-mem.md#criterios) | [falha](evaluations/claude-bedrock.md#criterios) | pendente | pendente |
| Privacidade antes da persistencia | [falha](evaluations/basic-memory.md#criterios) | [falha](evaluations/ai-memory.md#criterios) | [falha](evaluations/palinode.md#criterios) | [falha](evaluations/claude-mem.md#criterios) | [falha](evaluations/claude-bedrock.md#criterios) | [falha](evaluations/agent-memory.md#criterios) | [falha](evaluations/twiceshy.md#criterios) |
| Sem persistencia de sessoes | [falha](evaluations/basic-memory.md#criterios) | [falha](evaluations/ai-memory.md#criterios) | [falha](evaluations/palinode.md#criterios) | [falha](evaluations/claude-mem.md#criterios) | [falha](evaluations/claude-bedrock.md#criterios) | [falha](evaluations/agent-memory.md#criterios) | [falha](evaluations/twiceshy.md#criterios) |
| Estado atual | [parcial](evaluations/basic-memory.md#criterios) | [parcial](evaluations/ai-memory.md#criterios) | pendente | [falha](evaluations/claude-mem.md#criterios) | pendente | pendente | [falha](evaluations/twiceshy.md#criterios) |
| Consolidacao antes da publicacao | [falha](evaluations/basic-memory.md#criterios) | [parcial](evaluations/ai-memory.md#criterios) | [parcial](evaluations/palinode.md#criterios) | [parcial](evaluations/claude-mem.md#criterios) | [parcial](evaluations/claude-bedrock.md#criterios) | [parcial](evaluations/agent-memory.md#criterios) | [parcial](evaluations/twiceshy.md#criterios) |
| Revisao explicita | [falha](evaluations/basic-memory.md#criterios) | [parcial](evaluations/ai-memory.md#criterios) | [parcial](evaluations/palinode.md#criterios) | [falha](evaluations/claude-mem.md#criterios) | [parcial](evaluations/claude-bedrock.md#criterios) | [parcial](evaluations/agent-memory.md#criterios) | [parcial](evaluations/twiceshy.md#criterios) |
| Repositorio de memoria separado | [passa](evaluations/basic-memory.md#criterios) | [passa](evaluations/ai-memory.md#criterios) | [passa](evaluations/palinode.md#criterios) | [parcial](evaluations/claude-mem.md#criterios) | [passa](evaluations/claude-bedrock.md#criterios) | [falha](evaluations/agent-memory.md#criterios) | [passa](evaluations/twiceshy.md#criterios) |
| Integracao com Claude Code | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Testabilidade | pendente | pendente | pendente | pendente | [falha](evaluations/claude-bedrock.md#criterios) | pendente | pendente |
| Licenca compativel | [falha](licenses.md#basic-memory) | [parcial](licenses.md#ai-memory) | [parcial](licenses.md#palinode) | [parcial](licenses.md#claude-mem) | [parcial](licenses.md#claude-bedrock) | [parcial](licenses.md#agent-memory) | [falha](licenses.md#twiceshy) |
| Custo de adaptacao | [passa](evaluations/basic-memory.md#criterios) | [passa](evaluations/ai-memory.md#criterios) | [passa](evaluations/palinode.md#criterios) | [passa](evaluations/claude-mem.md#criterios) | [passa](evaluations/claude-bedrock.md#criterios) | [passa](evaluations/agent-memory.md#criterios) | [passa](evaluations/twiceshy.md#criterios) |

## Disposicao

| Projeto | Disposicao inicial | Disposicao preliminar | Relatorio |
|---|---|---|---|
| Basic Memory | referencia | [referencia preliminar](evaluations/basic-memory.md#recomendacao-preliminar) | `evaluations/basic-memory.md` |
| ai-memory | comparador end-to-end | [reimplementacao conceitual preliminar](evaluations/ai-memory.md#recomendacao-preliminar) | `evaluations/ai-memory.md` |
| Palinode | componente ou referencia | [componente preliminar, condicionado a spike](evaluations/palinode.md#recomendacao-preliminar) | `evaluations/palinode.md` |
| claude-mem | referencia | [referencia preliminar](evaluations/claude-mem.md#recomendacao-preliminar) | `evaluations/claude-mem.md` |
| claude-bedrock | referencia ou componente | [reimplementacao conceitual preliminar](evaluations/claude-bedrock.md#recomendacao-preliminar) | `evaluations/claude-bedrock.md` |
| agent-memory | componente | [componente preliminar, condicionado a spike](evaluations/agent-memory.md#recomendacao-preliminar) | `evaluations/agent-memory.md` |
| twiceshy | referencia | [referencia preliminar](evaluations/twiceshy.md#recomendacao-preliminar) | `evaluations/twiceshy.md` |

`parcial` na linha de licenca significa que a licenca raiz e permissiva, mas o componente exato, suas dependencias e seus ativos ainda nao foram auditados. Para Basic Memory e twiceshy, a falha se aplica a fork e reutilizacao direta sob a premissa de manter o produto Apache-2.0; ambos continuam validos como referencias conceituais.

As disposicoes acima permanecem preliminares. A etapa 3 foi executada nos candidatos principais Basic Memory e ai-memory; valores `pendente` identificam os demais fluxos comportamentais limitados e permanecem abertos para as etapas seguintes.
