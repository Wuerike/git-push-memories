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
- um repositório de memória em um remote Git (GitHub);
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
curl -fsSL https://raw.githubusercontent.com/Wuerike/git-push-memories/main/install.sh | sh
```

Para instalar diretamente a partir de um clone do repositorio, com Go disponivel:

```bash
make install
```

Por padrao, o binario e instalado em `~/.local/bin`. Use `make install INSTALL_DIR=/outro/diretorio` para escolher outro destino.

### Configuracao

A memoria vive em um repositorio Git remoto (GitHub para o `/promote`; qualquer URL Git serve para o resto). Crie um repositorio vazio e configure-o uma unica vez, em cada maquina:

```bash
gpm setup git@github.com:org/memoria.git --categories domains,decision
```

O setup e idempotente. Em um repositorio vazio ele cria `gpm.json` (`{"schema_version": 1, "memory_categories": [...]}`, categorias unicas em lowercase kebab-case), a `main` e a `proposals`; em um repositorio que ja tem `gpm.json` ele so clona e valida; se o repositorio tem conteudo sem `gpm.json`, falha. `--yes` roda sem prompts (obrigatorio fora de um terminal; com repositorio vazio exige `--categories`).

O GPM mantem um clone bare gerenciado em `$XDG_STATE_HOME/git-push-memories/<id>` e nunca toca em nenhum repositorio seu. O setup tambem registra o MCP e instala `/remember` e `/promote` no escopo global do Claude Code. Pre-requisitos: `git` com worktree, `claude`, identidade Git (`user.name` e `user.email`) e credenciais Git para o remote. O `gh` instalado e autenticado so e exigido pelo `/promote`.

Para forcar squash nos PRs de release e proteger a `main`, veja [docs/workflow.md](docs/workflow.md): ative a protecao da `main` **depois** do setup, porque o push inicial seria bloqueado.

### Fluxo

Tres branches (modelo completo em [docs/workflow.md](docs/workflow.md)):

- `main`: memoria canonica (`memories/<categoria>/**/*.md`) e `gpm.json`; protegida, so recebe PR de release;
- `proposals`: `main` mais as propostas pendentes (`proposals/<id>-<slug>.md`); o `/remember` escreve e envia aqui;
- `release/<opened_at>`: onde o `/promote` consolida todas as propostas; vira o PR para a `main`.

```text
/remember <fato>     # cria uma proposta e a envia para a branch proposals
/promote             # abre o release, consolida todas as propostas em lotes e abre o PR
```

Depois do merge do PR (squash recomendado), o proximo `/promote` ou `gpm promote --finish` finaliza o release. Release abandonado: `gpm promote --resume` ou `gpm promote --abort`.

Memorias canonicas: Markdown na raiz, arquivos diretos em `memories/`, categorias nao configuradas e `proposals/` sao ignorados pela busca. A leitura sempre usa a `main` publicada, atualizada com `fetch` no maximo a cada 10 minutos.

O servidor MCP publica estas tools:

- `memory_search`, `memory_expand` e `memory_page_read`, para buscar secoes, expandir por handle (detectando conteudo obsoleto) e ler uma pagina inteira com seu content hash; aceitam `source: release` durante o `/promote`;
- `memory_reference_snapshot` e `memory_reference_expand`, para identificar e revisar blobs commitados sem persistir o checkout local;
- `memory_propose`, para registrar uma proposta Markdown v1/v2 na `proposals` e envia-la (`pushed`), ou retornar a proposta byte-identica ja existente;
- `memory_proposal_list`, para listar as propostas (`source: proposals`, ou `release`);
- `memory_promote_open`, `memory_promote_status` e `memory_promote_submit`, para abrir ou retomar o release, consultar o que falta e enviar para revisao (push da branch e PR via `gh`);
- `memory_promote_preview`, `memory_promote_apply`, `memory_promote_batch_preview` e `memory_promote_batch_apply`, para revisar o diff e aplicar de uma a dezesseis decisoes (`rewrite_page`, `fold_proposal`, descarte com motivo...) como um unico commit recuperavel na `release`.

O servidor e transitorio sobre stdio: o cliente MCP inicia o processo localmente e nenhuma porta, daemon ou container e necessario. Se uma promocao anterior foi interrompida, o servidor reconcilia seu marcador privado ao iniciar.

O [material de integracao com Claude Code](integrations/claude-code/README.md) descreve os detalhes. Nenhum hook de sessao e instalado e a captura nao altera memoria canonica. A mesma recuperacao pode ser chamada pela CLI:

```bash
gpm retrieve "checkout timeout team" --budget-bytes 2048
```

O preview separa candidatos canonicos, propostas relacionadas e integridade das referencias de fonte. `verified` significa apenas que lineage, commit, blob e hash correspondem em um checkout local vinculado; sustentacao semantica continua sendo avaliada separadamente como `supports`, `contradicts` ou `unclear`. Propostas byte-identicas sao deduplicadas; cada lote e um commit com recovery antes e depois do avanco da `release`.

O ciclo tecnico completo (setup, remember, promote, merge, finish, leitura) roda sem rede em remotos locais com `tests/e2e/run-phase-3.sh`. A run preserva somente evidencia sanitizada sob `.artifacts/` e executa `make verify` e `make verify-offline`.

## Licença

O software deste repositório é disponibilizado sob a [Apache License 2.0](LICENSE).
Essa licença não se estende automaticamente ao conteúdo dos repositórios de memória
criados ou mantidos pelos usuários.
