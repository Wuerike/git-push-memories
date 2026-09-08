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

Um time pode trabalhar em vários repositórios de código e manter um único repositório de memória. Cada pessoa configura a ferramenta com o caminho local desse repositório, previamente clonado.

Durante uma sessão, hooks e comandos identificam propostas de memória. A captura segue um modelo híbrido:

- o agente pode detectar conhecimento relevante automaticamente;
- a pessoa pode indicar explicitamente algo que deve ser lembrado.

As propostas passam por consolidação antes de se tornarem memória compartilhada. A revisão por pull request fará parte desse processo, junto de alguma forma de validação por agentes. A ordem, os critérios e os momentos dessas validações ainda precisam ser definidos a partir de casos de uso concretos.

## Princípios

### Estado atual acima do histórico

A memória deve explicar principalmente o estado atual do domínio. Ela não deve se tornar uma sequência extensa de deltas em que novas entradas apenas contradizem ou substituem entradas anteriores.

O histórico pode existir no próprio Git, mas não é o principal modelo de leitura da memória.

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

## Escopo inicial

O primeiro ciclo será validado com:

- um desenvolvedor de um time;
- um repositório de memória já clonado localmente;
- integração com Claude Code;
- armazenamento em Markdown e Git;
- operação sem backend.

Copilot CLI e OpenCode ficam fora do primeiro ciclo, mas continuam como integrações futuras desejadas.

## Sinais de sucesso

Após um período de uso, o produto deve demonstrar que:

- agentes recuperam contexto relevante para a tarefa atual;
- investigações e perguntas repetidas diminuem;
- decisões conhecidas deixam de ser rediscutidas sem necessidade;
- memórias incorretas ou contraditórias são raras;
- a manutenção manual da memória permanece baixa;
- a maioria das propostas geradas é relevante o suficiente para ser aproveitada.

## Questões em aberto

As seguintes decisões ainda não foram tomadas:

- quais casos de uso devem produzir uma proposta de memória;
- como identificar alterações, duplicidades e contradições;
- quando executar validações por agentes;
- quais critérios permitem aceitar, rejeitar ou pedir revisão de uma proposta;
- como combinar validação automática com revisão em pull request;
- como organizar e recuperar os arquivos Markdown;
- qual papel um índice local, como SQLite, terá na captura e na busca;
- como medir objetivamente relevância, correção e custo de manutenção.

Essas questões devem ser exploradas com cenários reais antes de serem transformadas em uma especificação detalhada.

## Pesquisa

- [Panorama de projetos de memória para IA](docs/ai-memory-projects-overview.md) — análise de projetos relacionados e recomendações para o primeiro ciclo.
- [Aderência do claude-bedrock](docs/bedrock-fit-analysis.md) — comparação entre o plugin público e o objetivo deste projeto.

## Planejamento

- [Roadmap](docs/roadmap.md) — sequência de resultados, avaliações e decisões que orientará o desenvolvimento.

## Desenvolvimento

O walking skeleton da Fase 2 requer Go 1.25.0 ou superior e fixa a toolchain Go 1.25.12. Para verificar o projeto:

```bash
make verify
```

Depois do primeiro download da toolchain e das dependências, `make verify-offline` repete a matriz sem acessar o proxy de módulos. Ambos os comandos impõem Go 1.25.12, inclusive quando outra versão está instalada localmente.

O [E2E sintetico da Fase 2](docs/phase-2/e2e-synthetic.md) executa o fluxo completo e uma passagem separada de privacidade, preservando evidencias sanitizadas em `.artifacts/phase-2/`.

O [experimento controlado de valor](docs/phase-2/value-experiment-protocol.md) usa somente fixtures sinteticos versionados e sessoes autenticadas isoladas para comparar tres pares com e sem memoria.

O [relatorio do experimento](docs/phase-2/value-experiment-report.md) registra a decisao pre-registrada `revisar o fluxo`: correcao foi preservada, mas o limiar por pares nao foi atingido com consistencia suficiente para iniciar a Fase 3.

## Instalacao

O primeiro release suporta Linux em `amd64` e `arm64`. A CLI pode ser instalada sem Go:

```bash
curl -fsSL https://raw.githubusercontent.com/Wuerike/git-push-memories/master/install.sh | sh
```

Para instalar diretamente a partir de um clone do repositorio, com Go disponivel:

```bash
make install
```

Por padrao, o binario e instalado em `~/.local/bin`. Use `make install INSTALL_DIR=/outro/diretorio` para escolher outro destino.

Configure uma unica vez um repositorio de memoria ja clonado e com pelo menos um commit:

```bash
gpm setup /path/to/memory-repository
```

O setup registra o MCP e instala `/remember` no escopo global do usuario. Depois disso, qualquer sessao do Claude Code pode executar `/remember <fato>`.

O servidor publica cinco tools:

- `memory_search`, para buscar excerpts Markdown dentro de um orçamento em bytes;
- `memory_expand`, para expandir uma seção por handle e detectar conteúdo obsoleto.
- `memory_propose_create_page`, `memory_propose_create_section` e `memory_propose_replace`, para criar propostas estruturadas sem aplicar Markdown ou executar Git.

O servidor é transitório sobre stdio: o cliente MCP inicia o processo localmente e nenhuma porta, daemon ou container é necessário.

O [material de integracao com Claude Code](integrations/claude-code/README.md) descreve os detalhes. Nenhum hook de sessao e instalado; apply e publish permanecem comandos humanos da CLI.

A mesma recuperação também pode ser chamada diretamente pela CLI:

```bash
gpm retrieve "checkout timeout team" --budget-bytes 2048
```

Propostas pendentes podem ser revisadas, rejeitadas ou aplicadas pela CLI com `proposal show`, `proposal reject` e `proposal apply`. Apply exige a revisão exata, por exemplo:

```bash
gpm proposal show <proposal-id>
gpm proposal apply <proposal-id> --revision 1
```

O apply deixa a atualização como `applied_unpublished`. A publicação local cria um commit limitado ao target sem consumir staging alheio:

```bash
gpm proposal publish <proposal-id>
```

O Publisher não acessa remotos. O commit recebe o trailer `Git-Push-Memories-Proposal: <id>/<revision>` e falhas recuperáveis preservam o estado `applied_unpublished`.

## Licença

O software deste repositório é disponibilizado sob a [Apache License 2.0](LICENSE).
Essa licença não se estende automaticamente ao conteúdo dos repositórios de memória
criados ou mantidos pelos usuários.
