# Roadmap do git-push-memories

Este roadmap descreve a direção de desenvolvimento do `git-push-memories`. Ele organiza o trabalho por resultados e decisões, sem antecipar uma arquitetura que ainda precisa ser validada.

O roadmap não é um backlog. Cada fase define o resultado esperado, os principais entregáveis e um gate de decisão. As tarefas executáveis devem ser criadas apenas para a fase atual, depois que seu escopo estiver suficientemente claro.

## Direção desejada

O primeiro objetivo é provar que uma memória compartilhada melhora o contexto fornecido a agentes em tarefas reais de engenharia. O produto deve validar um fluxo vertical completo:

```text
conhecimento descoberto
-> proposta de memória
-> consolidação do estado atual
-> revisão e publicação
-> recuperação em uma sessão futura
```

A solução poderá partir de um fork, reutilizar componentes existentes ou implementar conceitos observados em outros projetos. A escolha deve ser consequência dos casos de uso e das restrições do produto, não da quantidade de funcionalidades oferecidas por uma possível base.

## Restrições do primeiro ciclo

As decisões tomadas durante o roadmap devem preservar os princípios definidos no [README](../README.md):

- Markdown e Git como fonte canônica da memória compartilhada;
- funcionamento local, sem backend obrigatório;
- memória orientada ao estado atual do domínio;
- ausência de sessões completas, prompts e respostas na memória;
- captura automática e explícita;
- consolidação antes da publicação;
- integração inicial com Claude Code;
- validação inicial com um desenvolvedor e um repositório de memória já clonado.

Tecnologias como bancos vetoriais, grafos, ontologias e serviços externos não fazem parte do primeiro ciclo, salvo se um caso de uso demonstrar que são necessárias.

## Visão geral

| Fase | Resultado esperado | Decisão principal |
|---|---|---|
| 1. Avaliação de projetos | Referências concretas e escolha fundamentada entre fork, reutilização e implementação própria | Quais conceitos adotar e sobre qual base construir? |
| 2. Fluxo vertical mínimo | Conhecimento publicado em uma sessão é recuperado em outra | O fluxo produz valor? |
| 3. Curadoria e confiança | Propostas do inbox são consolidadas em memória canônica com segurança | O fluxo de curadoria é confiável? |
| 4. Captura automática | Hooks geram propostas úteis sem persistir sessões | A automação mantém qualidade e privacidade? |
| 5. Recuperação e escala | A busca continua relevante com o crescimento da memória | É necessário um índice mais sofisticado? |

As fases representam dependências de decisão, não necessariamente versões ou intervalos de tempo. Atividades preparatórias podem ocorrer em paralelo, mas nenhum gate deve ser considerado concluído sem evidência.

## Fase 1: Avaliação de projetos

### Resultado esperado

Uma decisão documentada sobre a base de implementação, fundamentada na observação de soluções existentes. O resultado pode ser fazer fork de um projeto, reutilizar componentes isolados ou construir uma implementação própria inspirada nas referências estudadas.

### Projetos selecionados para avaliação

A pesquisa em [Panorama de projetos de memória para IA](ai-memory-projects-overview.md) sugere avaliar os projetos por suas especialidades:

Os seguintes repositórios foram clonados em `~/workspace` para a avaliação da fase:

| Projeto | Repositório | Clone local | Aspecto a avaliar |
|---|---|---|---|
| Basic Memory | [basicmachines-co/basic-memory](https://github.com/basicmachines-co/basic-memory) | `~/workspace/basic-memory` | Markdown canônico e índice SQLite derivado |
| ai-memory | [akitaonrails/ai-memory](https://github.com/akitaonrails/ai-memory) | `~/workspace/ai-memory` | Fluxo integrado, fronteira de privacidade e consolidação |
| Palinode | [phasespace-labs/palinode](https://github.com/phasespace-labs/palinode) | `~/workspace/palinode` | Evidência, estados epistêmicos e operações estruturadas |
| claude-mem | [thedotmack/claude-mem](https://github.com/thedotmack/claude-mem) | `~/workspace/claude-mem` | Hooks do Claude Code e recuperação progressiva |
| claude-bedrock | [iurykrieger/claude-bedrock](https://github.com/iurykrieger/claude-bedrock) | `~/workspace/claude-bedrock` | Vault Markdown/Git, propostas de mudança e curadoria assistida por agentes |
| agent-memory | [xChuCx/agent-memory](https://github.com/xChuCx/agent-memory) | `~/workspace/agent-memory` | Diff, aplicação explícita, scan de segredos e revisão por Git |
| twiceshy | [dotts-h/twiceshy](https://github.com/dotts-h/twiceshy) | `~/workspace/twiceshy` | Quarentena, validação, promoção e revisão por pull request |

A lista é inicial. Um projeto pode ser removido cedo se sua licença, arquitetura ou modelo de dados for incompatível com as restrições do produto.

### Método de avaliação

1. Registrar versão, atividade, licença e obrigações de redistribuição de cada candidato.
2. Executar seus fluxos principais e rastrear onde os dados são capturados, persistidos, consolidados e recuperados.
3. Identificar, nos fluxos observados, abordagens úteis para relevância, correção, privacidade, consolidação e manutenção.
4. Criar spikes apenas para dúvidas que não possam ser respondidas pela documentação ou pelo código.
5. Comparar custo de adaptação, dependências operacionais, testabilidade e compatibilidade com as restrições do primeiro ciclo.
6. Registrar a decisão em um ADR antes de incorporar código ao produto.

### Formas de adoção

- **Fork:** quando um candidato já fornece a maior parte do núcleo desejado e sua arquitetura, licença e direção de manutenção são compatíveis.
- **Reutilização de componente:** quando uma capacidade está isolada, possui contrato claro e sua licença permite incorporação.
- **Reimplementação de conceito:** quando a ideia é útil, mas o código traz armazenamento, dependências ou decisões incompatíveis.
- **Referência:** quando o projeto contribui apenas com padrões, aprendizados ou exemplos.

O produto deve ter uma única base arquitetural principal. Capacidades de outros projetos devem ser incorporadas pelos contratos dessa base, evitando combinar modelos de persistência e ciclos de vida incompatíveis.

Todo código reutilizado deve ter origem, versão e licença registradas. Inspiração conceitual também deve ser referenciada quando influenciar uma decisão arquitetural relevante.

### Entregáveis

- matriz comparativa preenchida com evidências;
- análise de licenças e obrigações;
- spikes necessários e seus resultados;
- ADR com a escolha da base e alternativas rejeitadas;
- mapa das capacidades que serão adotadas, adaptadas ou implementadas.

### Gate de decisão

A fase termina quando existir uma base escolhida ou uma justificativa explícita para implementação própria, sem dúvidas críticas pendentes sobre licença ou compatibilidade arquitetural.

**Estado em 2026-09-05:** fase concluida pela [ADR 0001](adr/0001-implementation-base.md), que escolheu uma implementacao propria em Go com adaptacao seletiva do motor Markdown do agent-memory.

## Fase 2: Fluxo vertical mínimo

**Plano executável:** [Plano da Fase 2](phase-2/plan.md).

**Estado em 2026-09-06:** T1-T10 concluidas com decisao `revisar o fluxo`; fase reaberta para bootstrap sem Markdown manual e Fase 3 bloqueada ate nova validacao de valor.

### Resultado esperado

Demonstrar, de ponta a ponta, que conhecimento descoberto em uma sessão pode melhorar uma tarefa executada em outra sessão.

O primeiro fluxo deve privilegiar captura explícita para reduzir variáveis:

```text
comando explícito
-> pesquisa de memória relacionada
-> escolha confirmada entre criar página, criar seção ou substituir seção
-> proposta temporária
-> alteração de uma página Markdown
-> revisão local
-> publicação no repositório de memória
-> recuperação contextual em outra sessão
```

A proposta deve preferir alterar a explicação do estado atual a simplesmente acrescentar um novo registro cronológico. Quando ainda não houver alvo válido, o produto deve gerar página ou seção com identidade estável em vez de exigir preparação manual.

### Entregáveis

- integração mínima com Claude Code;
- comando de captura explícita;
- formato inicial das páginas e propostas;
- criação da primeira página e de novas seções com schema e anchors gerados pelo produto;
- escolha explícita entre criar página, criar seção e substituir o conteúdo de uma seção existente;
- consolidação de uma proposta como alteração Markdown;
- publicação usando o fluxo Git existente;
- recuperação com orçamento de contexto limitado;
- demonstração reproduzível aplicando os conceitos e critérios selecionados na fase 1.

### Gate de decisão

A fase termina quando o fluxo partir de um memory root sem Markdown preparado e ao menos um cenário autenticado mostrar que a memória publicada melhora a execução posterior sem exigir acesso à sessão original. O primeiro experimento, executado em 2026-09-06, teve 6/6 runs válidos, mas somente 1/3 pares superou o limiar de tempo; portanto o gate permanece aberto com decisão `revisar o fluxo`.

## Transição depois da Fase 2

**Estado em 2026-09-09:** a decisão `revisar o fluxo` levou à substituição do lifecycle técnico da Fase 2 pela [ADR 0004](adr/0004-markdown-proposal-inbox-and-memory-categories.md). Os planos, protocolos e relatórios da Fase 2 continuam sendo registros históricos do fluxo que avaliaram; não descrevem a interface atual.

A nova base mantém captura e curadoria separadas:

```text
captura explícita com /remember
-> busca nas categorias configuradas
-> confirmação humana
-> proposta Markdown no inbox
-> commit path-scoped diretamente em main
-> curadoria futura do inbox
-> memória canônica nas categorias configuradas
-> recuperação em sessões futuras
```

O produto atual entrega captura, recuperacao e os destinos individuais de curadoria. O repositório de memória declara categorias em `gpm.json`; a recuperação lê somente `memories/<categoria-configurada>/**/*.md`; propostas usam `proposals/<id>-<slug>.md`. Não existem branches de proposta, receipts, nem comandos `proposal show`, `proposal apply` ou `proposal publish`. O push permanece uma ação humana.

Essa mudança redefine o ponto de partida da Fase 3. O gate de valor da Fase 2 não foi convertido em aprovação retroativa e deverá ser reavaliado depois que existir um fluxo de curadoria utilizável.

## Fase 3: Curadoria e confiança

**Plano executável:** [Plano da Fase 3](phase-3/plan.md).

**Estado atual:** em andamento. Os destinos individuais, dedupe, evidencia de conflitos, referencias verificaveis e transacoes multi-path recuperaveis estao implementados; o E2E tecnico da Fase 3B forma o proximo incremento.

### Resultado esperado

Transformar propostas do inbox em memória canônica atual, coerente e recuperável, sem reintroduzir um lifecycle paralelo ao Git.

Uma curadoria concluída deve produzir um commit auditável que atualize os arquivos canônicos necessários e remova, no mesmo commit, todas as propostas consumidas. Propostas descartadas também devem sair do inbox por uma decisão Git observável. O desenho da interface de curadoria deve ser definido por tracer bullets antes de escolher comandos, automação ou política de branches.

### Fronteiras da fase

- O Markdown e o histórico Git continuam sendo a única autoridade durável; estado temporário de preview ou execução não cria um lifecycle paralelo.
- A ação e o target presentes em uma proposta são sugestões. A operação canônica é decidida durante a curadoria a partir da proposta e do estado atual.
- O agente pode classificar significado, duplicidade e contradição, mas não escreve memória por conta própria. O GPM valida estrutura, paths, hashes, privacidade, concorrência e o conjunto exato de mudanças.
- Nenhuma escrita ocorre antes da apresentação do diff e da confirmação explícita da pessoa.
- Um commit de curadoria contém as alterações canônicas e a remoção das propostas consumidas ou descartadas. Seus IDs devem permanecer identificáveis no histórico Git.
- Staging e mudanças alheias são preservados. O produto não executa fetch, pull, push, merge nem cria pull request.
- Sessões, prompts e respostas não são entradas da curadoria e não são persistidos.
- Captura automática, índice semântico e automação remota permanecem fora da fase.

As etapas abaixo são incrementos internos da Fase 3, não novas fases do roadmap. A Fase 3A corresponde ao primeiro tracer bullet prioritário; somente a conclusão da Fase 3C encerra a fase.

### Fase 3A: tracer bullet de curadoria

**Hipótese:** uma proposta explícita de criação pode se tornar memória canônica recuperável por um fluxo curto, revisável e seguro, sem exigir edição manual de Markdown.

**Cenário vertical:**

```text
proposta create no inbox
-> listar e selecionar a proposta
-> validar e ler seu conteúdo
-> escolher categoria e path canônico
-> gerar preview de criação de página
-> apresentar diff e pedir confirmação
-> revalidar proposta, HEAD, target e privacidade
-> criar memória e remover proposta
-> criar um commit contendo exatamente esses paths
-> recuperar a nova seção em outro processo
```

**Inclui:**

- parser e validação do schema Markdown vigente das propostas;
- interface mínima para listar, selecionar e visualizar uma proposta;
- operação inicial de criação de página, com IDs de seção gerados pelo produto;
- preview vinculado ao conteúdo da proposta, ao `HEAD` e ao post-state esperado;
- confirmação humana sobre o diff exato;
- escrita e commit path-scoped que não consomem staging alheio;
- remoção da proposta consumida no mesmo commit da memória criada;
- detecção de drift entre preview e apply;
- teste de ponta a ponta desde um memory root sem memória canônica até a recuperação em um novo processo.

**Fora de escopo:** atualização e remoção de memória existente, descarte, lotes de propostas, análise semântica de duplicidade ou contradição, captura automática e operações remotas.

**Critério de conclusão:** o cenário vertical é reproduzível; nenhuma recusa ou falha anterior à confirmação altera memória ou inbox; o commit aceito contém somente a página criada e a remoção da proposta; uma interrupção não pode publicar um commit parcial; e a memória criada é recuperável sem acesso à sessão original.

**Decisão habilitada:** confirmar se a interface e a fronteira entre agente, GPM, pessoa e Git são adequadas antes de expandir as operações de curadoria.

### Fase 3B: operações e confiança

**Hipótese:** o mesmo fluxo pode tratar todo o inbox e manter uma representação coerente do estado atual sem transformar análise probabilística em escrita automática.

**Inclui:**

- operações canônicas de criar página, criar seção, substituir seção e remover seção;
- descarte auditável de propostas sem alteração canônica;
- target preciso o suficiente para identificar a seção observada em updates e removes, incluindo path, section ID e sinal de drift quando aplicável;
- seleção de uma ou mais propostas e garantia de que todas as propostas selecionadas tenham um destino observável: consumir, descartar ou manter pendente sem alteração;
- deduplicação determinística de propostas idênticas e recuperação de candidatos relacionados no canônico e no inbox;
- análise assistida de possíveis equivalências e contradições sobre um conjunto limitado e explícito de candidatos;
- decisão humana para conflitos semânticos; similaridade ou contradição sugerida pelo agente não autoriza escrita por si só;
- definição não ambígua da origem de referências em cenários com múltiplos repositórios e validação determinística de existência, revisão e conteúdo quando a fonte estiver disponível localmente;
- distinção explícita entre integridade da referência, validável pelo GPM, e sustentação semântica da afirmação, avaliada pelo agente e pela pessoa;
- revalidação de schema, privacidade, targets, referências, hashes e `HEAD` imediatamente antes da escrita;
- commits seguros para múltiplos paths, com preservação de staging alheio, serialização local e diagnóstico de interrupções;
- testes de criação, atualização, remoção, descarte, duplicidade, contradição, referência inválida, drift, concorrência e privacidade.

**Fora de escopo:** decidir contradições de forma autônoma, provar semanticamente toda afirmação, executar código arbitrário como evidência, capturar sessões, acessar remotos ou introduzir banco vetorial.

**Critério de conclusão:** propostas válidas podem criar páginas ou seções, atualizar memória existente ou remover conhecimento de modo observável; propostas incorretas podem ser descartadas; duplicidades e contradições aparecem no preview e exigem decisão; referências inválidas não são apresentadas como verificadas; e nenhum commit aceito deixa no inbox uma proposta declarada como consumida.

**Decisão habilitada:** confirmar que o fluxo possui garantias suficientes para ser submetido a uma avaliação de valor com uso real.

### Fase 3C: fluxo compartilhado e avaliação de valor

**Hipótese:** a memória consolidada pelo fluxo vigente melhora uma tarefa futura e o custo de curadoria permanece baixo o suficiente para justificar captura automática posteriormente.

**Inclui:**

- documentação do fluxo completo de captura explícita, curadoria, inspeção do commit e push humano;
- definição do ponto de revisão compartilhada usando o fluxo Git do time, sem automação remota obrigatória;
- protocolo reproduzível que compare tarefas com e sem a memória canônica produzida pela curadoria atual;
- critérios definidos antes da execução para validade das runs e para a decisão do gate;
- medição de relevância recuperada, efeito na tarefa, exploração evitada, tempo de curadoria e volume de contexto;
- medição de propostas aceitas sem edição, aceitas com edição, descartadas, duplicadas ou contraditórias;
- registro de falhas de referência, conteúdo desatualizado e incidentes ou quase incidentes de privacidade;
- relatório com decisão explícita entre avançar para a Fase 4, revisar o fluxo ou interromper a expansão de escopo.

**Fora de escopo:** implementar hooks de captura automática durante o experimento ou adicionar um índice novo para compensar falhas sem antes registrá-las.

**Critério de conclusão:** o experimento é executado sobre o lifecycle vigente, seus resultados são reproduzíveis e existe uma decisão registrada a partir dos critérios previamente definidos.

**Decisão habilitada:** autorizar a Fase 4, revisar novamente o fluxo ou interromper a expansão de escopo.

### Gate de decisão

A fase termina somente depois da Fase 3C, quando propostas válidas puderem ser transformadas em memória canônica recuperável, propostas incorretas puderem ser descartadas, duplicidades e contradições forem tratadas de forma observável, e o processo inteiro mantiver histórico Git auditável sem deixar propostas consumidas no inbox. A avaliação de valor deve demonstrar resultado suficiente pelos critérios definidos no protocolo antes de autorizar a Fase 4. Caso contrário, a decisão permanece `revisar o fluxo` ou interrompe a expansão de escopo.

## Fase 4: Captura automática

### Resultado esperado

Reduzir a dependência de comandos manuais sem transformar sessões completas em memória nem gerar excesso de propostas irrelevantes.

### Abordagem

- observar eventos disponíveis nos hooks do Claude Code;
- extrair somente candidatos necessários para avaliar uma proposta;
- aplicar a fronteira de privacidade antes da persistência;
- produzir o mesmo schema de proposta do inbox e reutilizar o fluxo de curadoria da captura explícita;
- permitir que a pessoa desative, rejeite ou corrija a captura;
- medir aceitação, rejeição e motivos de descarte.

### Entregáveis

- hooks mínimos para geração de candidatos;
- regras iniciais sobre o que merece uma proposta;
- garantias de descarte dos dados temporários da sessão;
- métricas de relevância e custo de revisão;
- comparação entre captura explícita e automática.

### Gate de decisão

A fase termina quando a captura automática gerar propostas aproveitáveis com baixo custo de revisão e sem violar a política de privacidade.

## Fase 5: Recuperação e escala

### Resultado esperado

Manter a recuperação relevante, explicável e econômica conforme a memória cresce e passa a atender mais repositórios de código.

### Evolução orientada por necessidade

1. Começar com arquivos, metadados simples e busca textual.
2. Medir falhas de recuperação nos cenários reais.
3. Adotar SQLite ou FTS como índice local derivado se volume ou latência justificarem.
4. Considerar busca semântica, relações ou temporalidade somente para falhas que busca textual e estrutura de arquivos não resolvam.

Qualquer índice deve ser reconstruível a partir dos arquivos canônicos e não pode criar uma segunda fonte de verdade.

### Entregáveis

- benchmark reproduzível de recuperação;
- orçamento de contexto e estratégia de apresentação progressiva;
- índice local derivado, se justificado;
- processo de reconstrução e verificação do índice;
- avaliação com múltiplos repositórios de código usando a mesma memória.

### Gate de decisão

A fase termina quando a recuperação atender aos critérios definidos para relevância, latência e orçamento de contexto no volume testado.

## Evidências de progresso

O avanço não deve ser medido apenas por funcionalidades entregues. Cada fase deve preservar evidências relacionadas aos sinais de sucesso do produto:

- taxa de tarefas em que a memória recuperada foi relevante;
- diferença de resultado entre tarefas com e sem memória;
- quantidade de exploração ou perguntas evitadas;
- taxa de propostas aceitas, editadas e rejeitadas;
- incidência de duplicidades, contradições e conteúdo desatualizado;
- tempo gasto revisando e mantendo a memória;
- violações ou quase violações da fronteira de privacidade;
- volume de contexto fornecido ao agente.

Os valores aceitáveis serão definidos a partir das evidências produzidas ao longo das fases. Métricas sem uma decisão associada não devem orientar complexidade adicional.

## Do roadmap para tarefas

Somente a fase ativa deve ser decomposta em tarefas detalhadas. Cada tarefa deve declarar:

- cenário ou hipótese que pretende avaliar;
- escopo e itens explicitamente excluídos;
- artefato que será produzido;
- critério observável de conclusão;
- dependências e decisão que seu resultado permitirá tomar.

Por exemplo, "analisar o Basic Memory" não é uma tarefa suficientemente definida. Uma tarefa executável seria:

> Executar o fluxo de recuperação do Basic Memory, registrando como o projeto organiza o Markdown e o índice SQLite, a qualidade dos resultados, as dependências e o esforço de adaptação às restrições do produto.

## Documentos de apoio

- O [README](../README.md) descreve o problema, o objetivo e os princípios do produto.
- Este roadmap descreve a sequência de resultados e decisões.
- ADRs registrarão escolhas arquiteturais relevantes e alternativas rejeitadas.
- Issues ou tickets descreverão o trabalho executável da fase atual.
- Relatórios de spikes preservarão evidências temporárias usadas nas decisões.
