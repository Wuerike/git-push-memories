# Resumo da Etapa 7

## Resultado

Dois spikes capazes de alterar a escolha arquitetural foram executados e passaram. Tres possibilidades foram encerradas sem execucao porque seus resultados nao mudariam a decisao atual.

| Candidato | Decisao da Etapa 7 | Disposicao para a Etapa 8 |
|---|---|---|
| agent-memory | separacao de roots e viavel com seam e correcoes delimitadas | componente, condicionado a endurecimento |
| Palinode | parser/executor e evidencias podem ser extraidos offline | candidato a componente para a fatia isolada |
| claude-mem | spike nao aberto | referencia e reimplementacao conceitual |
| Basic Memory | spike de licenca nao aberto | referencia; nenhuma copia direta |
| ai-memory | spike generico nao aberto | reimplementacao conceitual; sanitizador condicional |

## Spikes executados

- [Roots separados no agent-memory](agent-memory-split-roots.md): passou com dois code roots somente leitura, memoria compartilhada, concorrencia e Git confinado. O resultado nao corrige as falhas de privacidade e governanca da revisao original.
- [Extracao offline do Palinode](palinode-offline-extraction.md): passou com uma fatia exata e uma facade pequena, sem backend, sessao, LLM ou rede. Proveniencia composta e transacao multi-arquivo continuam fora da fatia.

## Spikes nao abertos

### claude-mem

O comportamento util de recuperacao continua acoplado ao worker, endpoints HTTP e `memory_session_id`; remover esses elementos substituiria o modelo demonstrado em vez de extrai-lo. Reabrir somente se TypeScript/Node for escolhido, houver uma interface Markdown/Git estavel e uma lista exata de arquivos cuja reutilizacao mudaria o mapa de capacidades.

### Basic Memory

A linha `MIT` no README do plugin nao delimita um grant para a revisao, e o comportamento relevante reside no nucleo AGPL. Reabrir somente se existir intencao de copiar uma lista fechada de arquivos fora de `src/basic_memory/`, acompanhada por esclarecimento formal do mantenedor e revisao juridica.

### ai-memory

O daemon, o store e o modelo episodico continuam centrais ao fluxo amplo. O sanitizador de `ai-memory-core` e uma possibilidade isolada, mas nao justifica spike antes de Rust ser escolhido e um pacote exato entrar na shortlist. Se isso ocorrer, executar auditoria de dependencia/licenca ou um spike restrito ao contrato de sanitizacao antes da primeira persistencia.

## Entrada para a Etapa 8

A Etapa 8 deve comparar uma unica base arquitetural principal e evitar combinar dois modelos de operacoes. Ela deve decidir entre o nucleo do agent-memory e a fatia do Palinode para cada seam, registrar componentes exatos e licencas, preencher os criterios ainda pendentes conforme o papel de cada candidato e criar o mapa de capacidades.
