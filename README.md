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
- [Comparação dos mecanismos de geração de memória](docs/memory-generation-mechanisms-comparison.md) — análise do pipeline atual e das sete referências, com implicações prováveis para a qualidade da memória.
- [Aderência do claude-bedrock](docs/bedrock-fit-analysis.md) — comparação entre o plugin público e o objetivo deste projeto.

## Desenvolvimento

O projeto requer Go 1.25.0 ou superior e fixa a toolchain Go 1.25.12. Para verificar o projeto:

```bash
make verify
```

Depois do primeiro download da toolchain e das dependências, `make verify-offline` repete a matriz sem acessar o proxy de módulos. Ambos os comandos impõem Go 1.25.12, inclusive quando outra versão está instalada localmente.

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

O repositorio de memoria deve ter pelo menos um commit, estar em `main` para criar propostas e conter `gpm.json` na raiz:

```json
{
  "schema_version": 1,
  "memory_categories": ["domains", "decision"]
}
```

As categorias devem ser unicas e usar lowercase kebab-case. Configure o repositorio uma unica vez:

```bash
gpm setup /path/to/memory-repository
```

O setup registra o MCP e instala `/remember` e `/curate` no escopo global do usuario. Depois disso, qualquer sessao do Claude Code pode propor um fato e consumir ou descartar uma proposta.

Memorias canonicas ficam em `memories/<categoria-configurada>/**/*.md`. Markdown na raiz, arquivos diretos em `memories/`, categorias nao configuradas e `proposals/` sao ignorados pela busca. Propostas ficam em `proposals/<id>-<slug>.md` e recebem um commit path-scoped diretamente na `main`, sem consumir staging alheio.

O servidor publica onze tools:

- `memory_search`, para buscar secoes nas categorias configuradas dentro de um orçamento em bytes;
- `memory_expand`, para expandir uma seção por handle e detectar conteúdo obsoleto;
- `memory_reference_snapshot`, para identificar blobs commitados por lineage, revisao, path e hash sem persistir o checkout local;
- `memory_reference_expand`, para revisar linhas limitadas dos bytes commitados identificados sem persistir o conteudo;
- `memory_propose`, para registrar uma proposta Markdown v1/v2 ou retornar a proposta byte-identica ja existente sem novo commit;
- `memory_proposal_list`, para listar propostas commitadas e as categorias configuradas;
- `memory_page_read`, para ler uma pagina canonica inteira com seu content hash (ou o erro `budget_too_small`), base de `rewrite_page`;
- `memory_curate_preview`, para gerar sem escrita o diff e evidencia separada de candidatos canonicos e propostas relacionadas;
- `memory_curate_apply`, para aplicar uma unica vez um preview emitido pelo mesmo processo MCP;
- `memory_curate_batch_preview`, para revisar de uma a dezesseis decisoes como uma unica transacao, incluindo `rewrite_page` (reescrita da pagina inteira) e `fold_proposal` (proposta absorvida pela reescrita ou pela criacao de pagina do mesmo lote);
- `memory_curate_batch_apply`, para aplicar o lote confirmado em um unico commit recuperavel.

O servidor é transitório sobre stdio: o cliente MCP inicia o processo localmente e nenhuma porta, daemon ou container é necessário. Se uma curadoria anterior foi interrompida, o servidor reconcilia seu marcador privado antes de aceitar chamadas.

O [material de integracao com Claude Code](integrations/claude-code/README.md) descreve os detalhes. Nenhum hook de sessao e instalado e a captura nao altera memoria canonica. A curadoria processa uma proposta ou um lote limitado de targets distintos e exige confirmacao do diff antes de alterar memoria ou descartar propostas.

A mesma recuperação também pode ser chamada diretamente pela CLI:

```bash
gpm retrieve "checkout timeout team" --budget-bytes 2048
```

Depois da confirmacao em `/remember`, a proposta ja esta legivel no worktree e commitada localmente em `main`. O produto nao acessa remotos; o push permanece humano:

```bash
git -C <memory-repository> show --stat HEAD
git -C <memory-repository> push
```

A curadoria e iniciada em uma sessao do Claude Code:

```text
/curate [proposal-id]
```

Sem ID, a skill lista o inbox para a pessoa selecionar ate dezesseis propostas. O preview separa candidatos canonicos, propostas relacionadas e integridade das referencias de fonte. `verified` significa apenas que lineage, commit, blob e hash correspondem em um checkout local vinculado; sustentacao semantica continua sendo avaliada separadamente como `supports`, `contradicts` ou `unclear`. Propostas byte-identicas sao deduplicadas no inbox; lotes usam targets distintos, um commit e recovery antes/depois do avanco de `main`.

O lifecycle tecnico completo pode ser reproduzido em repositorios descartaveis com `tests/e2e/run-phase-3.sh`. A run preserva somente evidencia sanitizada sob `.artifacts/`, executa `make verify` e `make verify-offline` e nao acessa remotos.

## Licença

O software deste repositório é disponibilizado sob a [Apache License 2.0](LICENSE).
Essa licença não se estende automaticamente ao conteúdo dos repositórios de memória
criados ou mantidos pelos usuários.
