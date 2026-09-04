# Matriz comparativa da Fase 1

Esta matriz e o indice das classificacoes. O significado dos valores e as condicoes de aceitacao estao no [protocolo de avaliacao](evaluation-protocol.md#escala-de-classificacao). Cada valor diferente de `pendente` deve apontar para evidencia no relatorio do candidato ou no registro de licencas da fase.

| Criterio | Basic Memory | ai-memory | Palinode | claude-mem | claude-bedrock | agent-memory | twiceshy |
|---|---|---|---|---|---|---|---|
| Markdown e Git canonicos | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Operacao local | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Ausencia de backend obrigatorio | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Indice reconstruivel | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Privacidade antes da persistencia | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Sem persistencia de sessoes | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Estado atual | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Consolidacao antes da publicacao | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Revisao explicita | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Repositorio de memoria separado | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Integracao com Claude Code | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Testabilidade | pendente | pendente | pendente | pendente | pendente | pendente | pendente |
| Licenca compativel | [falha](licenses.md#basic-memory) | [parcial](licenses.md#ai-memory) | [parcial](licenses.md#palinode) | [parcial](licenses.md#claude-mem) | [parcial](licenses.md#claude-bedrock) | [parcial](licenses.md#agent-memory) | [falha](licenses.md#twiceshy) |
| Custo de adaptacao | pendente | pendente | pendente | pendente | pendente | pendente | pendente |

## Disposicao

| Projeto | Disposicao inicial | Disposicao final | Relatorio |
|---|---|---|---|
| Basic Memory | referencia | pendente | `evaluations/basic-memory.md` |
| ai-memory | comparador end-to-end | pendente | `evaluations/ai-memory.md` |
| Palinode | componente ou referencia | pendente | `evaluations/palinode.md` |
| claude-mem | referencia | pendente | `evaluations/claude-mem.md` |
| claude-bedrock | referencia ou componente | pendente | `evaluations/claude-bedrock.md` |
| agent-memory | componente | pendente | `evaluations/agent-memory.md` |
| twiceshy | referencia | pendente | `evaluations/twiceshy.md` |

`parcial` na linha de licenca significa que a licenca raiz e permissiva, mas o componente exato, suas dependencias e seus ativos ainda nao foram auditados. Para Basic Memory e twiceshy, a falha se aplica a fork e reutilizacao direta sob a premissa de manter o produto Apache-2.0; ambos continuam validos como referencias conceituais.
