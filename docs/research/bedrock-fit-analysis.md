# Aderência do claude-bedrock ao git-push-memories

**Data da análise: 4 de setembro de 2026.**

Esta análise utiliza informações disponíveis no repositório público [`iurykrieger/claude-bedrock`](https://github.com/iurykrieger/claude-bedrock).

## Resumo executivo

O `claude-bedrock` tem alta aderência às escolhas de Markdown, Git, captura explícita e curadoria assistida por agentes. Sua aderência é menor ao diferencial central do `git-push-memories`: capturar conhecimento descoberto durante sessões de programação, consolidá-lo como estado atual e recuperá-lo como contexto em tarefas futuras.

A recomendação é manter o `git-push-memories` como projeto independente e avaliar o `claude-bedrock` como referência ou fonte de componentes isolados sob licença MIT, não como fork para o núcleo do produto.

| Dimensão | Aderência |
|---|---|
| Markdown e Git canônicos | Alta |
| Operação local sem backend | Alta |
| Integração com Claude Code | Alta |
| Captura explícita | Alta |
| Propostas antes da escrita | Alta |
| Captura automática de descobertas da sessão | Baixa |
| Consolidação orientada ao estado atual | Baixa a média |
| Validações determinísticas | Baixa |
| Portabilidade entre agentes | Baixa |

## O que é o claude-bedrock

O projeto público se apresenta como um plugin para Claude Code que transforma um vault Obsidian em um second brain estruturado. Sua arquitetura é composta principalmente por:

- skills procedurais em Markdown executadas pelo Claude Code;
- definições semânticas de entidades;
- templates de frontmatter e conteúdo;
- Markdown e Git como armazenamento;
- Graphify e Docling para extração e ingestão;
- Obsidian como principal experiência humana;
- um hook `Stop` em Python para relatar erros do próprio plugin.

O fluxo mais próximo do objetivo deste projeto é o `/bedrock:preserve`: ele procura entidades existentes, apresenta uma proposta, aguarda confirmação e publica alterações usando uma estratégia Git configurável. A recuperação ocorre por `/bedrock:ask`, combinando grafo, busca textual e limite de entidades lidas.

O repositório usa a [licença MIT](https://github.com/iurykrieger/claude-bedrock/blob/main/LICENSE). Ela permite uso, modificação e redistribuição desde que o aviso de copyright e a licença sejam preservados em cópias ou partes substanciais.

## Comparação com os objetivos atuais

| Critério do git-push-memories | claude-bedrock | Aderência |
|---|---|---|
| Markdown e Git como fonte canônica | É a fundação da solução | Alta |
| Memória compartilhada | Um vault pode ser compartilhado via Git | Alta |
| Operação local sem backend obrigatório | O armazenamento é local, embora alguns fluxos dependam de ferramentas externas | Alta |
| Integração inicial com Claude Code | É específico para Claude Code | Alta |
| Captura explícita | `/preserve` e `/learn` atendem bem | Alta |
| Captura automática durante trabalho de programação | O hook existente trata telemetria de erros, não propostas de memória | Baixa |
| Recuperação orientada à tarefa atual | Existe consulta explícita, mas não injeção automática no ciclo da sessão | Média |
| Memória orientada ao estado atual | Algumas entidades aceitam substituição; outras seguem regras append-only | Baixa a média |
| Duplicidades e contradições | Há matching e compressão, mas a execução depende principalmente de prompts | Média |
| Propostas revisáveis e pull requests | Existe confirmação e estratégia opcional de PR; push direto é o padrão | Média |
| Privacidade das sessões | Sessões não são a memória canônica, mas o hook de erros lê o transcript e usa telemetria opt-out | Média |
| Validações determinísticas | A maior parte das regras não possui implementação determinística | Baixa |
| Portabilidade futura entre agentes | Skills e ferramentas são específicas do Claude Code | Baixa |
| Primeiro ciclo sem grafo obrigatório | Graphify e Docling ocupam papel central no setup e na ingestão | Baixa |

## Ideias úteis

1. **Separar ferramenta e dados.** O software pode ser público enquanto cada repositório de memória pertence a seu usuário ou time.
2. **Manter Markdown como fonte canônica.** Índices e grafos devem ser derivados e reconstruíveis.
3. **Propor antes de escrever.** Captura não deve significar publicação automática.
4. **Centralizar a consolidação.** Captura explícita, automática e importação devem convergir para o mesmo contrato de proposta.
5. **Registrar proveniência.** Uma memória precisa indicar origem e data de verificação.
6. **Separar candidatos de memória canônica.** Conteúdo incompleto não deve ser publicado como fato.
7. **Diagnosticar a saúde da memória.** Links quebrados, schemas inválidos e conhecimento obsoleto precisam ser detectáveis.
8. **Limitar contexto na recuperação.** Busca progressiva evita enviar o vault inteiro ao agente.

## Diferenças que justificam outro projeto

1. **A unidade de valor é contexto para uma tarefa.** Páginas e documentação são consequências da memória, não o objetivo final.
2. **A fonte principal são descobertas durante sessões de engenharia.** Ingestão de documentos externos é secundária no primeiro ciclo.
3. **O estado atual deve substituir afirmações obsoletas.** O histórico já permanece disponível no Git.
4. **Governança deve fazer parte do núcleo.** Proposta, diff, validação, rejeição e revisão precisam de estados observáveis.
5. **Claude Code deve ser um adaptador.** O modelo de memória não deve impedir futuras integrações com outros agentes.

Essas diferenças definem uma proposta própria: **memória de domínio Git-first, extraída do trabalho de engenharia, consolidada para representar o estado atual e publicada com revisão**.

## Estratégia de reutilização

### Usar como referência

- separação entre conteúdo temporário e canônico;
- metadados de proveniência;
- proposta de mudança antes da aplicação;
- healthcheck do repositório;
- estratégias configuráveis de publicação Git;
- recuperação com orçamento limitado de contexto.

### Avaliar como componente MIT

- estrutura de skills para o primeiro adaptador Claude Code;
- convenções genéricas de proveniência;
- partes do hook e de seus testes, caso exista uma necessidade equivalente;
- adaptadores de documentos, somente se ingestão externa entrar no escopo.

Qualquer cópia substancial deve manter o aviso MIT e ter origem e versão registradas.

### Não adotar como fundação

- a taxonomia inteira antes de validar casos reais;
- Obsidian como requisito do produto;
- Graphify ou Docling como dependências obrigatórias no primeiro ciclo;
- grafo como índice primário antes de medir busca textual;
- regras append-only para memória canônica;
- validação e transações Git implementadas apenas por prompts;
- push direto como padrão para memória compartilhada;
- telemetria opt-out que lê resultados da sessão;
- identidade baseada somente em nomes de arquivos e wikilinks.

## Recomendação

1. Manter o `git-push-memories` como projeto independente.
2. Classificar o `claude-bedrock` como referência e possível fonte de componentes pontuais, não como base para fork.
3. Validar primeiro captura explícita, proposta temporária, consolidação do estado atual, revisão Git e recuperação em outra sessão.
4. Usar corpus sintético ou projetos públicos no desenvolvimento e nos testes do projeto open source.
5. Registrar qualquer componente MIT incorporado e preservar os avisos exigidos.
6. Distribuir o código do GPM sob a [licença Apache-2.0](../../LICENSE),
   preservando os avisos dos componentes de terceiros incorporados.

## Fontes públicas

- [README](https://github.com/iurykrieger/claude-bedrock/blob/main/README.md)
- [Licença MIT](https://github.com/iurykrieger/claude-bedrock/blob/main/LICENSE)
- [Regras e modelo do vault](https://github.com/iurykrieger/claude-bedrock/blob/main/CLAUDE.md)
- [Skill de preservação](https://github.com/iurykrieger/claude-bedrock/blob/main/skills/preserve/SKILL.md)
- [Skill de recuperação](https://github.com/iurykrieger/claude-bedrock/blob/main/skills/ask/SKILL.md)
- [Hook Stop](https://github.com/iurykrieger/claude-bedrock/blob/main/hooks/hooks.json)
