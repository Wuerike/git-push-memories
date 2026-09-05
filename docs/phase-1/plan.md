# Plano de execução da Fase 1

Este documento detalha a execução da [Fase 1: Avaliação de projetos](../roadmap.md#fase-1-avaliação-de-projetos). O objetivo da fase é escolher, com evidências, entre fazer fork de um projeto, reutilizar componentes isolados ou construir uma implementação própria inspirada nas referências avaliadas.

## Resultado esperado

Uma decisão documentada sobre a base de implementação do `git-push-memories`, sem dúvidas críticas pendentes sobre licença ou compatibilidade arquitetural.

## Premissas

- O `git-push-memories` continuará distribuído sob Apache License 2.0.
- Projetos AGPL podem ser avaliados como referências, mas não serão candidatos a fork ou reutilização direta sem uma decisão explícita de relicenciamento.
- Todos os testes usarão dados sintéticos e ambientes descartáveis.
- Nenhuma sessão real, credencial ou informação privada será usada como evidência.
- Os candidatos serão avaliados nas revisões locais já clonadas. As revisões devem ser registradas antes da execução e não serão atualizadas durante a comparação.
- Spikes serão criados somente quando documentação, testes e código não forem suficientes para responder a uma dúvida capaz de alterar a decisão.

## Artefatos da fase

```text
docs/phase-1/
├── plan.md
├── evaluation-protocol.md
├── comparison-matrix.md
├── licenses.md
├── capability-map.md
├── evaluations/
│   └── <project>.md
└── spikes/
    └── <spike>.md

docs/adr/
└── 0001-implementation-base.md
```

Cada relatório de avaliação deve registrar:

- projeto, repositório, revisão e data da avaliação;
- hipótese avaliada;
- escopo e itens excluídos;
- comandos executados e ambiente utilizado;
- evidências produzidas;
- aderência aos critérios comuns;
- riscos, limitações e custo de adaptação;
- disposição recomendada: fork, componente, reimplementação conceitual, referência ou rejeição.

## Corpus e cenário comum

Todos os projetos devem usar, quando suas interfaces permitirem, o mesmo cenário sintético:

1. A memória inicial informa que o timeout de `checkout-api` é de cinco segundos e que o serviço pertence ao `team-a`.
2. Uma sessão posterior descobre que o timeout atual é de três segundos e que o serviço pertence ao `team-b`.
3. A descoberta deve produzir uma proposta, não uma publicação silenciosa.
4. A consolidação deve substituir o estado antigo em vez de criar duas afirmações igualmente autoritativas.
5. Uma nova sessão deve responder corretamente qual é o timeout e quem é o responsável, sem acessar a sessão original.
6. A remoção de qualquer índice derivado não pode remover a memória canônica; o índice deve ser reconstruível a partir do Markdown.

O corpus também terá credenciais e dados pessoais explicitamente sintéticos para verificar a fronteira de privacidade. Os valores devem ser reconhecíveis como marcadores de teste e nunca corresponder a segredos reais.

## Critérios de avaliação

Cada candidato será classificado como `passa`, `parcial`, `falha` ou `não se aplica` nos critérios abaixo:

| Critério | Evidência esperada |
|---|---|
| Markdown e Git canônicos | A memória permanece íntegra e legível sem o índice ou o processo da ferramenta |
| Operação local | O fluxo principal funciona sem serviço remoto obrigatório |
| Ausência de backend obrigatório | Não é necessário manter daemon ou infraestrutura adicional para o fluxo mínimo |
| Índice reconstruível | A remoção do índice seguida de reconstrução preserva os resultados esperados |
| Privacidade antes da persistência | Marcadores sensíveis são descartados antes da primeira escrita em disco |
| Sem persistência de sessões | Prompts, respostas, transcrições e referências de sessão não são armazenados na memória |
| Estado atual | Uma descoberta nova substitui ou corrige o conhecimento anterior |
| Consolidação antes da publicação | A descoberta passa por uma representação temporária antes de alterar o Markdown canônico |
| Revisão explícita | A pessoa consegue inspecionar, ajustar, rejeitar e aceitar a proposta |
| Repositório de memória separado | Um repositório de memória pode servir a um ou mais repositórios de código |
| Integração com Claude Code | O fluxo pode ser acionado e recuperado no ciclo de vida do Claude Code |
| Testabilidade | As partes críticas possuem contratos e testes reproduzíveis |
| Licença compatível | Fork ou reutilização é compatível com Apache 2.0 e suas obrigações estão registradas |
| Custo de adaptação | Mudanças necessárias são identificadas e estimadas qualitativamente |

Uma falha estrutural pode eliminar um candidato como base principal sem impedir que conceitos específicos sejam estudados. A eliminação deve indicar quais capacidades ainda merecem avaliação.

## Candidatos e disposição inicial

| Projeto | Revisão inicial | Disposição inicial | Avaliação principal |
|---|---|---|---|
| Basic Memory | `a55003e7` | Referência por causa da AGPL | Markdown canônico, SQLite derivado e recuperação |
| ai-memory | `3bd35154` | Comparador end-to-end MIT | Captura, privacidade, consolidação, reindexação e recuperação |
| Palinode | `9832d586` | Componente ou referência MIT | Evidências, estados epistêmicos e operações estruturadas |
| claude-mem | `be44b6c8` | Referência Apache 2.0 | Hooks do Claude Code e recuperação progressiva |
| claude-bedrock | `7cac16ce` | Referência ou componente MIT | Proposta, revisão Git, publicação e recuperação explícita |
| agent-memory | `51dac9bf` | Candidato a componentes Apache 2.0 | Diff, aplicação explícita e scan de segredos |
| twiceshy | `aa8f1d88` | Referência por causa da AGPL | Quarentena, validação, promoção e governança |

As revisões acima são os pontos de partida identificados durante o planejamento. Devem ser confirmadas e registradas junto do estado da árvore de trabalho no início de cada avaliação.

## Sequência de execução

### Etapa 1: preparar o protocolo

1. Criar `evaluation-protocol.md` com o cenário comum, comandos de coleta, convenções de evidência e escala de classificação.
2. Criar `comparison-matrix.md` com todos os critérios e candidatos.
3. Criar `licenses.md` com revisão, licença, obrigações e formas permitidas de adoção.
4. Preparar um repositório Git descartável de memória e um repositório descartável de código.
5. Registrar versões de Git, Claude Code, runtimes e ferramentas usadas.

**Conclusão observável:** outro avaliador consegue reproduzir o cenário comum e preencher um relatório sem precisar definir novos critérios.

### Etapa 2: realizar a triagem estática

1. Confirmar licença, revisão, atividade e stack de cada candidato.
2. Rastrear no código onde dados são recebidos, filtrados, persistidos, consolidados e recuperados.
3. Identificar dependências obrigatórias, armazenamento canônico e modelo de publicação.
4. Aplicar os critérios eliminatórios que independem de execução.
5. Registrar capacidades que continuam relevantes mesmo quando o projeto é eliminado como base.

**Conclusão observável:** cada projeto tem uma disposição preliminar justificada e uma lista limitada de fluxos que ainda precisam ser executados.

### Etapa 3: avaliar armazenamento e recuperação

Executar principalmente em Basic Memory e ai-memory:

1. Criar e indexar as páginas do corpus comum.
2. Recuperar a informação inicial e registrar resultado, latência e volume de contexto.
3. Alterar diretamente o Markdown para representar o novo estado.
4. Confirmar que somente o estado atual é recuperado.
5. Remover o índice derivado em uma cópia descartável.
6. Reconstruir o índice apenas a partir dos arquivos canônicos.
7. Repetir a recuperação e comparar os resultados.
8. Inspecionar o Git diff e qualquer dado adicional persistido.

**Conclusão observável:** está demonstrado se o Markdown é suficiente para reconstruir a recuperação e quais dependências são necessárias para isso.

### Etapa 4: avaliar consolidação do estado atual

Executar principalmente em ai-memory, Palinode e claude-bedrock:

1. Semear uma página com o estado inicial.
2. Apresentar a descoberta que substitui timeout e ownership.
3. Capturar a proposta ou operação gerada antes da aplicação.
4. Inspecionar tratamento de duplicidade, contradição, evidência e proveniência.
5. Aplicar a mudança em ambiente descartável.
6. Verificar se o estado anterior deixou de ser autoritativo.
7. Repetir a operação para verificar idempotência.

**Conclusão observável:** cada candidato demonstra se atualiza uma representação do estado atual ou acumula registros cronológicos e contraditórios.

### Etapa 5: avaliar governança

Executar principalmente em agent-memory e claude-bedrock:

1. Criar uma proposta sem alterar a memória canônica.
2. Exibir o diff proposto.
3. Ajustar e rejeitar propostas.
4. Aplicar explicitamente uma proposta válida.
5. Repetir com segredo sintético e confirmar que não houve persistência.
6. Alterar o arquivo alvo entre proposta e aplicação para testar detecção de drift.
7. Inspecionar staging, commit, branch e arquivos não relacionados.
8. Exercitar branch e pull request em repositório descartável, quando a capacidade existir.

**Conclusão observável:** está documentado onde termina a proposta, quando o conteúdo canônico muda e quais garantias existem contra segredo, drift e alterações não relacionadas.

### Etapa 6: avaliar integração com Claude Code

Executar em claude-mem, agent-memory e ai-memory:

1. Instalar a integração em configuração isolada do Claude Code.
2. Observar eventos de hooks e dados enviados por cada evento.
3. Capturar uma descoberta sintética em uma sessão descartável.
4. Encerrar a sessão e aguardar o processamento previsto.
5. Iniciar uma sessão nova e recuperar a descoberta.
6. Medir contexto injetado e chamadas necessárias para chegar ao conteúdo completo.
7. Inspecionar arquivos, bancos, logs e requisições para identificar material de sessão persistido.
8. Repetir com o processo auxiliar indisponível para observar o comportamento de falha.

**Conclusão observável:** estão demonstrados a utilidade dos hooks, o custo de contexto, a degradação em caso de falha e os riscos de privacidade.

**Estado em 2026-09-05:** etapa encerrada como `parcial` nos tres candidatos previstos. claude-mem demonstrou hooks, injecao compacta e recuperacao progressiva em harness; ai-memory entregou handoff, mas nao teve resposta autenticada nem recuperacao MCP completa; agent-memory carregou skill/MCP e completou o ciclo funcional, mas nao possui hooks e depende de obediencia do modelo. Nenhuma passagem usou o sandbox Bubblewrap integral, e os eventos posteriores a falha de autenticacao foram dirigidos por harnesses deterministas. Os relatorios registram essas limitacoes, degradacao e persistencia observadas.

### Etapa 7: executar spikes condicionais

Os seguintes spikes só devem ser abertos se a resposta puder alterar a escolha arquitetural:

| Spike potencial | Decisão habilitada |
|---|---|
| Separar `code_root` e `memory_root` no agent-memory | Determinar se ele pode fornecer o núcleo de proposta e aplicação |
| Extrair operações do Palinode sem daemon ou índice vetorial | Determinar se consolidação e evidências podem ser reutilizadas como componente |
| Isolar hooks e recuperação do claude-mem sem worker, telemetria ou sessões | Determinar se há componente reutilizável para o adaptador Claude Code |
| Esclarecer a licença do plugin Claude Code do Basic Memory | Determinar se qualquer código do plugin pode ser reutilizado |
| Operar componentes do ai-memory sem daemon e memória episódica | Determinar se ele pode ser adaptado em vez de apenas referenciado |

Cada spike terá timebox de um dia útil e deverá produzir:

- pergunta binária ou decisão claramente delimitada;
- protótipo descartável ou evidência de código;
- resultado e limitações;
- recomendação de continuar ou abandonar;
- atualização da matriz comparativa.

### Etapa 8: consolidar resultados

1. Preencher a matriz com links para todas as evidências.
2. Revisar obrigações de licença e atribuição de qualquer capacidade candidata à reutilização.
3. Criar `capability-map.md` classificando cada capacidade como adotar, adaptar, reimplementar, referenciar ou rejeitar.
4. Comparar custo de adaptação, dependências operacionais e riscos das alternativas restantes.
5. Registrar dúvidas remanescentes e encerrar as que não alteram a decisão.

O mapa deve incluir, no mínimo:

- formato e organização do Markdown;
- índice local e reconstrução;
- contrato de proposta;
- consolidação do estado atual;
- validação e privacidade;
- diff e aplicação;
- publicação Git e pull request;
- hooks do Claude Code;
- recuperação progressiva;
- proveniência e evidência.

### Etapa 9: registrar a decisão

Criar `docs/adr/0001-implementation-base.md` contendo:

- contexto e restrições;
- alternativas consideradas;
- evidências determinantes;
- decisão entre fork, reutilização de componentes e implementação própria;
- base arquitetural escolhida;
- capacidades adotadas de cada referência;
- consequências, riscos e obrigações de licença;
- alternativas rejeitadas e motivos;
- implicações para o fluxo vertical da Fase 2.

A hipótese inicial é uma implementação própria com conceitos e, quando apropriado, componentes isolados. Ela não deve ser tratada como decisão antes da conclusão das avaliações.

## Ordem e paralelismo

A sequência crítica é:

```text
protocolo
-> triagem estática
-> fluxos comparáveis
-> spikes decisivos
-> matriz e mapa de capacidades
-> ADR
```

Depois da definição do protocolo, avaliações independentes podem ocorrer em paralelo. Avaliações que compartilham o mesmo corpus ou ambiente devem preservar configurações isoladas para impedir interferência entre resultados.

O ADR depende da matriz, do mapa de capacidades e do encerramento dos spikes decisivos. Nenhum código dos candidatos deve ser incorporado ao produto antes da aprovação do ADR.

## Evidências mínimas

Para cada execução, preservar:

- revisão, branch, remoto e estado da árvore de trabalho;
- versões das ferramentas e dependências relevantes;
- configuração efetiva com dados sensíveis removidos;
- comandos, códigos de saída e duração;
- árvore de arquivos antes e depois;
- hashes e Git diffs do Markdown;
- resultados de busca e volume aproximado de contexto;
- inventário de arquivos, tabelas e logs persistidos;
- resultado da busca por marcadores sintéticos sensíveis;
- saída dos testes relevantes;
- avaliação qualitativa vinculada aos critérios comuns.

Logs brutos não devem conter sessões completas. Quando uma ferramenta produzir esse conteúdo, o relatório deve registrar sua existência e manter apenas extratos sanitizados necessários para comprovar o comportamento.

## Gate de conclusão

A Fase 1 termina quando:

- todos os candidatos possuem uma disposição final justificada;
- a matriz comparativa está preenchida com evidências reproduzíveis;
- licenças, atribuições e obrigações estão documentadas;
- os fluxos relevantes foram executados ou dispensados com justificativa estática;
- todos os spikes capazes de alterar a decisão estão encerrados;
- o mapa de capacidades está completo;
- o ADR escolhe uma base ou justifica explicitamente uma implementação própria;
- não existem dúvidas críticas pendentes sobre licença ou compatibilidade arquitetural.

## Estimativa

Estimativa para uma pessoa: de oito a doze dias úteis.

| Atividade | Estimativa |
|---|---:|
| Protocolo, corpus, matriz e registro de licenças | 1 dia |
| Triagem estática e relatórios iniciais | 1 a 2 dias |
| Armazenamento, reindexação e recuperação | 1 a 2 dias |
| Consolidação e governança | 2 a 3 dias |
| Integração com Claude Code e privacidade | 1 a 2 dias |
| Spikes condicionais | 0 a 2 dias |
| Consolidação, mapa de capacidades e ADR | 1 dia |

A estimativa não constitui prazo da fase. A conclusão é determinada pelo gate e pelas evidências, não pelo tempo transcorrido.
