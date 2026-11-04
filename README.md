# git-push-memories

`git-push-memories` é um projeto open source para criar e manter uma memória compartilhada de domínio a partir de sessões de trabalho com agentes de programação.

As integrações com Claude Code e Codex oferecem as skills `remember` e `promote`,
apoiadas por ferramentas MCP em Go para acessar o repositório de memória, Git
e GitHub. No Claude Code, invoque `/remember` e `/promote`; no Codex,
`$remember` e `$promote`. As skills interpretam o conhecimento e conduzem
as decisões de conteúdo.

## Problema

Times de engenharia acumulam decisões, regras de negócio, fatos, lógicas e conhecimento sobre funcionalidades enquanto trabalham. Parte desse conhecimento não está explícita no código ou fica dispersa entre pessoas e sessões de agentes.

Sem uma memória compartilhada, agentes e pessoas precisam repetir investigações, fazer novamente as mesmas perguntas e podem tomar decisões incorretas por falta de contexto.

## Objetivo

O objetivo operacional do produto é fornecer aos agentes o contexto correto para executar tarefas.

A documentação do domínio e a preservação de decisões são consequências desse objetivo. Espera-se reduzir:

- perguntas repetidas;
- exploração desnecessária do código;
- rediscussão de decisões já tomadas;
- decisões incorretas causadas por contexto ausente ou desatualizado.

## Visão de uso

Um time pode trabalhar em vários repositórios de código e manter um único repositório de memória. Cada pessoa configura a ferramenta com a URL Git desse repositório.

Durante uma sessão, `remember` investiga conhecimento relacionado e apresenta
uma proposta de texto livre para confirmação. A captura envia o Markdown para
a branch `proposals` do repositório compartilhado.

`promote` seleciona propostas, investiga memórias relacionadas e discute um
plano com a pessoa. Depois da aprovação, o agente edita arquivos Markdown
comuns, apresenta o diff e pede aprovação para abrir um PR. A pessoa faz o merge;
a conclusão limpa apenas os insumos selecionados que continuam com o mesmo
conteúdo. As categorias de `gpm.json` são as opções de destino para o agente.

## Princípios

### Estado atual acima do histórico

A memória deve explicar principalmente o estado atual do domínio. Ela não deve se tornar uma sequência extensa de deltas em que novas entradas apenas contradizem ou substituem entradas anteriores.

### Repositório como memória compartilhada

As memórias ficam em arquivos Markdown em um repositório Git compartilhado.
A busca textual retorna caminhos, linhas e uma revisão Git para o agente
investigar o conhecimento publicado.

### Sem backend obrigatório

O produto deve funcionar localmente, integrado às ferramentas de desenvolvimento e ao fluxo Git existente do time.

### Privacidade das sessões

Sessões completas, prompts, respostas e referências para sessões não devem ser armazenados na memória.

### Captura explícita

A pessoa invoca `remember` e confirma o texto antes do compartilhamento.
O agente usa a conversa e a investigação de conhecimento existente para
preparar uma proposta que faça sentido fora da sessão.

### Consolidação antes da publicação

Uma proposta passa por revisão para se tornar memória. Na consolidação,
o agente investiga duplicidades, inconsistências, contradições
e alterações em conhecimento existente, com revisão e aprovação da pessoa.

## Documentação

- [Funcionalidades](docs/features.md) — comandos, skills, ferramentas MCP e requisitos de operação.
- [Fluxo de uso](docs/workflow.md) — branches, worktrees, setup, captura, consolidação e finalização.
- [Integração com Claude Code](integrations/claude-code/README.md) — configuração e uso das skills.
- [Integração com Codex](integrations/codex/README.md) — setup, skills e acesso aos arquivos.
- [Glossário](CONTEXT.md) — termos do domínio.
- Pesquisa:
  - [Panorama de projetos de memória para IA](docs/research/ai-memory-projects-overview.md)
  - [Comparação dos mecanismos de geração de memória](docs/research/memory-generation-mechanisms-comparison.md)
  - [Aderência do claude-bedrock](docs/research/bedrock-fit-analysis.md)

## Instalação

O primeiro release suporta Linux em `amd64` e `arm64`:

```bash
curl -fsSL https://raw.githubusercontent.com/Wuerike/git-push-memories/main/install.sh | sh
```

Ou, a partir de um clone com Go disponível, `make install` (use `INSTALL_DIR=/outro/diretorio` para mudar o destino, padrão `~/.local/bin`).

Depois, uma vez por máquina, aponte para o repositório de memória. Para Codex:

```bash
gpm setup git@github.com:org/memoria.git --agent codex --categories domains,decision
```

Para Claude Code, use `--agent claude-code` ou omita `--agent`. Configure cada
integração com a mesma URL para usar os dois agentes.

Pré-requisitos, efeitos do setup e uso no dia a dia: [features](docs/features.md),
[integração com Codex](integrations/codex/README.md) e
[integração com Claude Code](integrations/claude-code/README.md).

## Desenvolvimento

O projeto requer Go 1.25.0 ou superior e fixa a toolchain Go 1.25.12.

```bash
make verify          # testes, race, vet e build
make verify-offline  # idem, sem acessar o proxy de módulos
```

Os comandos impõem Go 1.25.12, inclusive quando outra versão está instalada. O E2E sem rede roda com `tests/e2e/run-phase-3.sh`.

## Licença

O software deste repositório é disponibilizado sob a [Apache License 2.0](LICENSE).
Essa licença não se estende automaticamente ao conteúdo dos repositórios de memória
criados ou mantidos pelos usuários.
