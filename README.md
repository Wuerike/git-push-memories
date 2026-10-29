# git-push-memories

`git-push-memories` é um projeto open source para criar e manter uma memória compartilhada de domínio a partir de sessões de trabalho com agentes de programação.

A ferramenta será integrada inicialmente ao Claude Code e deverá transformar conhecimento descoberto durante o trabalho em memória reutilizável por sessões futuras. Integrações com Copilot CLI e OpenCode poderão ser adicionadas posteriormente.

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

Durante uma sessão, hooks e comandos identificam propostas de memória. A captura segue um modelo híbrido:

- o agente pode detectar conhecimento relevante automaticamente;
- a pessoa pode indicar explicitamente algo que deve ser lembrado.

As propostas passam por consolidação antes de se tornarem memória compartilhada. A revisão por pull request fará parte desse processo, junto de alguma forma de validação por agentes. A ordem, os critérios e os momentos dessas validações ainda precisam ser definidos a partir de casos de uso concretos.

## Princípios

### Estado atual acima do histórico

A memória deve explicar principalmente o estado atual do domínio. Ela não deve se tornar uma sequência extensa de deltas em que novas entradas apenas contradizem ou substituem entradas anteriores.

### Repositório como memória compartilhada

As memórias serão mantidas como arquivos Markdown em um repositório Git compartilhado. Armazenamento local, como SQLite, poderá apoiar indexação e busca sem exigir um backend.

### Sem backend obrigatório

O produto deve funcionar localmente, integrado às ferramentas de desenvolvimento e ao fluxo Git existente do time.

### Privacidade das sessões

Sessões completas, prompts, respostas e referências para sessões não devem ser armazenados na memória.

### Captura híbrida

A ferramenta combina descoberta automática com captura explicitamente solicitada pela pessoa. O objetivo é integrar a construção da memória ao trabalho sem depender apenas da disciplina manual e sem entregar todo o controle ao agente.

### Consolidação antes da publicação

Uma proposta não é automaticamente uma memória válida. Antes da consolidação, será necessário considerar duplicidades, inconsistências, contradições e possíveis alterações em conhecimento existente.

## Documentação

- [Funcionalidades](docs/features.md) — comandos, skills, tools MCP, restrições e o que falta.
- [Fluxo de uso](docs/workflow.md) — branches, worktrees, setup, `/remember`, `/promote` e finalização.
- [Integração com Claude Code](integrations/claude-code/README.md) — configuração e uso das skills.
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

Depois, uma vez por máquina, aponte para o repositório de memória:

```bash
gpm setup git@github.com:org/memoria.git --categories domains,decision
```

Pré-requisitos, efeitos do setup e uso no dia a dia: [features](docs/features.md) e [integração com Claude Code](integrations/claude-code/README.md).

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
