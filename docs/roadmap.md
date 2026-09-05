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
| 3. Confiança e governança | Propostas são consolidadas e publicadas com segurança | O fluxo é confiável? |
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

### Resultado esperado

Demonstrar, de ponta a ponta, que conhecimento descoberto em uma sessão pode melhorar uma tarefa executada em outra sessão.

O primeiro fluxo deve privilegiar captura explícita para reduzir variáveis:

```text
comando explícito
-> proposta temporária
-> alteração de uma página Markdown
-> revisão local
-> publicação no repositório de memória
-> recuperação contextual em outra sessão
```

A proposta deve preferir alterar a explicação do estado atual a simplesmente acrescentar um novo registro cronológico.

### Entregáveis

- integração mínima com Claude Code;
- comando de captura explícita;
- formato inicial das páginas e propostas;
- consolidação de uma proposta como alteração Markdown;
- publicação usando o fluxo Git existente;
- recuperação com orçamento de contexto limitado;
- demonstração reproduzível aplicando os conceitos e critérios selecionados na fase 1.

### Gate de decisão

A fase termina quando ao menos um cenário real mostrar que a memória publicada melhora a execução posterior sem exigir acesso à sessão original.

## Fase 3: Confiança e governança

### Resultado esperado

Tornar o fluxo mínimo seguro o suficiente para produzir memória compartilhada revisável.

### Capacidades

- detecção de duplicidades e possíveis contradições;
- atualização ou remoção de conhecimento desatualizado;
- validações determinísticas de estrutura e referências;
- validação por agente nos pontos em que ela agregar evidência;
- fronteira de privacidade antes de qualquer persistência;
- apresentação clara do diff proposto;
- criação de branch e suporte à revisão por pull request;
- estados e motivos explícitos para aceitar, rejeitar ou solicitar revisão.

### Entregáveis

- política inicial de consolidação;
- modelo de estados de uma proposta;
- conjunto de validações e seus limites;
- testes para privacidade, duplicidade, contradição e atualização;
- fluxo documentado de revisão e publicação.

### Gate de decisão

A fase termina quando uma proposta incorreta, contraditória ou sensível for interrompida ou encaminhada para revisão, e uma proposta válida puder ser publicada com histórico auditável.

## Fase 4: Captura automática

### Resultado esperado

Reduzir a dependência de comandos manuais sem transformar sessões completas em memória nem gerar excesso de propostas irrelevantes.

### Abordagem

- observar eventos disponíveis nos hooks do Claude Code;
- extrair somente candidatos necessários para avaliar uma proposta;
- aplicar a fronteira de privacidade antes da persistência;
- reutilizar o mesmo fluxo de consolidação e revisão da captura explícita;
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
