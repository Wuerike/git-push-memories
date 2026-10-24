# Fluxo de uso do GPM

Este documento define como o GPM move conhecimento de uma sessão de trabalho até a memória compartilhada: captura (`/remember`), consolidação (`/promote`) e leitura.

## Modelo

O repositório de memória tem duas branches de longa vida e uma de curta vida:

| Branch | Conteúdo | Quem escreve | Proteção |
|---|---|---|---|
| `main` | Somente memória canônica (`memories/**`) e `gpm.json` | Merge de PR de release | Protegida (exige PR) |
| `proposals` | Memória da `main`, propostas pendentes (`proposals/<id>-<slug>.md`) e, durante um release, o marcador `release.lock.json` | `/remember` e o ciclo do release | Sem proteção |
| `release/<opened_at>` | Checkout da `proposals`, onde as propostas são consolidadas. O nome é único por release (o marcador guarda em `release_branch`). Nesta documentação, "a `release`" é essa branch | `/promote` | Existe no máximo uma por vez |

Princípios:

- **A `main` é a versão publicada.** Tudo que foi revisado e mergeado está nela. Não há tags: o SHA da `main` identifica a versão.
- **Propostas nunca entram na `main`.** O inbox vive na `proposals`, o que evita que `/remember` esbarre na proteção da `main` e mantém a `main` sem propostas pendentes.
- **A `proposals` nunca é rebaseada.** Ela só recebe merges (de `origin/main` e de `origin/proposals`), porque rebase refaz os merges de `origin/main` como commits novos e depois conflita.
- **O lock do release vive na `proposals`.** Um arquivo marcador na própria branch indica que há um release em andamento e bloqueia outro até a finalização. O lock não depende de a branch `release` existir, então apagá-la, manualmente ou pela forge, não o libera.
- **A `main` também aceita PRs manuais.** Corrigir uma memória por fora do fluxo é válido. A `proposals` acompanha a `main` pelo merge de `origin/main` no `Open` e na finalização (ver "Finalizar o release").
- **O PR de release deve ser mergeado com squash (boa prática).** Assim o histórico da `main` não carrega os commits que adicionaram e removeram propostas, nem o texto de propostas descartadas (`sensitive`, `incorrect`), e a diferença líquida do PR é só memória. O GPM funciona com qualquer método de merge que o repositório permitir (squash, merge comum ou rebase): a finalização não depende do método, usa o estado do PR e confere que o commit de merge está em `origin/main`. O custo dos outros métodos é só um histórico mais poluído.
- **Leitura sempre sobre a `main`.** Propostas não são lidas pela busca. A única exceção é o `/promote`, que lê a `release` explicitamente (ver "Consolidar").

## Repositório e worktrees

Há um único repositório de memória por pessoa, sempre um repositório do GitHub. O GPM mantém um clone **bare** dele no diretório de estado, sem nenhuma branch em checkout, e toda leitura e escrita acontece em worktrees sobre esse clone:

| Worktree | Branch | Uso |
|---|---|---|
| `read` | `main` (detached em `origin/main`) | `memory_search`, `memory_expand`, `retrieve` |
| `proposals` | `proposals` | `/remember` |
| `release` | `release/<opened_at>` | `/promote`, criado sob demanda e removido ao fim |

O GPM não toca em nenhum repositório da pessoa. Antes de qualquer escrita, o GPM normaliza o worktree: um merge, rebase ou cherry-pick pendente (por exemplo, de um Sync ou Finalize interrompido) é abortado, porque essas operações são idempotentes e se refazem. Se o worktree estiver sujo por outro motivo, o comando falha com mensagem clara em vez de forçar.

Os commits do GPM ignoram a configuração Git da pessoa: sem hooks, sem assinatura, sem template e com identidade explícita.

A resolução dos paths fica em um único ponto (`internal/config`). O restante do código recebe o path e não assume layout de pastas nem nome de branch.

## Remote e offline

O remote (`origin`, no GitHub) é requisito do GPM.

- `/remember` faz push na `proposals`. A confirmação do `/remember` já avisa que a proposta será enviada ao repositório compartilhado.
- A leitura faz `fetch` periódico.
- `/promote` grava o marcador de release com push na `proposals` (o lock) e abre o PR.
- Os comandos do `gh` sempre levam `-R owner/repo`, e o `gh pr create` leva `--title` e `--body` (nunca interativo).
- Operação rejeitada por falta de permissão é erro.
- Toda chamada de rede tem timeout, e o estouro conta como falha de rede. Falha de rede não perde dados: o `/remember` commita localmente, avisa e reenvia na próxima chamada. A leitura usa o último SHA conhecido.

## Setup

`gpm setup <url>` é o único comando de preparação. É idempotente: a cada execução ele calcula o que falta a partir do estado atual e completa só isso, então serve para o primeiro uso, para outra máquina e para reparo.

Para começar a usar o GPM, a pessoa cria um repositório vazio no GitHub e roda `gpm setup` com a URL dele.

**Entradas:**

- URL do repositório (obrigatória). Aceita qualquer URL Git; só o `/promote` exige que seja GitHub (via `gh`). A URL diferente da configurada é erro, sem normalização (`git@github.com:o/r` e `https://github.com/o/r` contam como diferentes).
- Categorias de memória (`--categories` ou prompt), necessárias só quando o repositório é criado. Com `--yes` e sem `--categories`, o comando falha pedindo a flag.
- `--yes`, para execução sem prompts. Sem TTY e sem `--yes`, o comando falha pedindo a flag.

**Pré-requisitos:** `git` com suporte a worktree, CLI do `claude`, identidade Git (`user.name` e `user.email`) e credenciais Git da pessoa para o remote. O GPM não gerencia credenciais. O `gh` instalado e autenticado só é exigido no primeiro `/promote` (verificado com `gh auth status`); quem só lê e faz `/remember` não precisa dele.

**Resolução da origem:**

| Situação | Ação |
|---|---|
| URL de remote vazio | Cria `gpm.json`, `main` e `proposals` e envia |
| URL com `gpm.json` na `main` | Clona, valida o `gpm.json` e usa a `proposals` do remote; se ela não existir, cria a partir da `main` e envia |
| URL com conteúdo e sem `gpm.json` na `main` | Erro |

**Etapas:**

1. Se já houver repositório configurado e a URL for diferente, falha. Para trocar de repositório, a pessoa remove o diretório de estado e roda o setup de novo.
2. Resolve a origem conforme a tabela acima. O diretório do repositório é derivado da URL.
3. Valida que a `main` contém um `gpm.json` válido e pelo menos um commit.
4. Cria os worktrees `read` e `proposals`.
5. Registra o MCP e instala as skills `/remember` e `/promote` no escopo global do usuário.
6. Grava a configuração do usuário (URL do repositório e diretório de estado).

**Squash (opcional):** para forçar o squash, a pessoa configura o repositório para permitir só esse tipo de merge: `Settings → General → Pull Requests` (marcar apenas "Allow squash merging"), ou um ruleset na `main` com "Allowed merge methods = squash". Exige permissão de administrador e vale para todo PR na `main`. O GPM não checa nem configura isso.

**Proteção da `main`:** ela e as permissões no remote ficam fora do alcance do GPM. A `main` deve exigir PR para que a revisão do release tenha efeito. A pessoa deve ativá-la **depois** do setup, porque o push inicial na `main` de um repositório vazio é bloqueado por ela. Se o push inicial for rejeitado, o setup falha com mensagem explicando isso.

## Leitura

- O worktree `read` acompanha a `main` com `git fetch --prune` e `checkout --detach origin/main`, no máximo uma vez a cada 10 minutos (TTL). Não há processo separado em segundo plano.
- No MCP, o comando de leitura responde na hora com o que o `read` já tem e, se o TTL venceu, dispara a atualização numa tarefa paralela dentro do próprio processo. O lock do repositório só é tomado no `checkout`, não durante a rede. No `gpm retrieve`, a atualização é síncrona.
- A hora da tentativa é gravada mesmo quando o `fetch` falha, então sem rede só a primeira leitura depois do TTL tenta atualizar. Sem rede, a leitura usa o último SHA conhecido e segue funcionando.
- Uma mudança na `main` pode levar até o TTL (mais uma leitura) para aparecer. O `/promote` e o `--finish` sempre fazem `fetch`, sem olhar o TTL, e atualizam o `read` ao finalizar, então quem acabou de mergear o release vê a memória nova na hora.
- Uma busca de vários arquivos que coincida com a atualização pode misturar versões. O risco é aceito.
- Se a `main` avançar no meio de uma sessão, um handle de `memory_search` pode apontar para conteúdo que mudou. O `memory_expand` detecta isso pelo hash e informa que o conteúdo está obsoleto.

## `/remember`

1. Faz `fetch` da `proposals`, para que a deduplicação byte-idêntica veja o que já foi enviado.
2. Cria a proposta em `proposals/<id>-<slug>.md` no worktree `proposals` e faz um commit path-scoped.
3. Faz push. Em caso de rejeição, faz `fetch` e `merge --no-edit origin/proposals` (nunca `pull`, que obedece ao `pull.rebase` da pessoa) e tenta de novo. Cada proposta é um arquivo único, então conflitos não são esperados. Se houver conflito, faz `merge --abort`, mantém a proposta commitada localmente e informa o erro.
4. Se o push falhar por falta de rede, a proposta permanece commitada localmente e é enviada na próxima chamada.

Nenhuma etapa do `/remember` toca a `main`.

## `/promote`

`/promote` consolida **todas** as propostas pendentes em memória canônica e termina com a pasta `proposals/` vazia na `release`.

### O marcador do release

`release.lock.json`, na raiz da `proposals`, existe somente enquanto há um release em andamento. Conteúdo:

- `release_branch`: nome da branch do release;
- `base`: commit da `proposals` do qual a `release` foi cortada;
- `proposals`: IDs das propostas incluídas no release;
- `opened_by` e `opened_at`, este em UTC compacto (`20261023T100000Z`), formato válido em nome de branch e usado direto em `release/<opened_at>`.

O marcador não guarda o PR: o PR é sempre achado pela branch do release. O marcador nunca entra na `release`, porque ela é cortada do commit anterior a ele.

### 1. Abrir o release

1. Faz `fetch --prune` da `proposals`. Se o marcador existir e o release já tiver sido mergeado, finaliza-o antes (ver "Finalizar o release"); se isso falhar, o comando para. Se o marcador continuar existindo, o comando falha e informa autor e data. Se existir `release` local (worktree e branch) e o marcador não existir ou apontar outra `release_branch`, remove o worktree e a branch locais e roda `git worktree prune` (resto de um release finalizado ou abortado em outra máquina).
2. Anota o `HEAD` local da `proposals` e faz merge de `origin/main` nela, para que a memória dela esteja em dia.
3. Cria um commit que adiciona o marcador, com `base` no commit anterior, e faz push na `proposals`. Se o push for rejeitado porque alguém empurrou antes (detectado por `fetch` e checagem de que `origin/proposals` não é ancestral do `HEAD`), faz `reset --hard` para o `HEAD` anotado no passo 2, o que descarta só o merge e o commit do marcador e preserva propostas locais ainda não enviadas, e repete do passo 1. Esse push é o lock: quem chegar depois encontra o marcador.
4. Cria localmente a branch `release/<opened_at>` em `base` e o worktree `release`. A branch só é enviada ao remote no envio para revisão.

As propostas existentes em `base` são as deste release, e o marcador as lista. Propostas que chegarem depois ficam só na `proposals`, para o próximo.

### 2. Consolidar

- Durante a consolidação, as ferramentas de leitura (`memory_search`, `memory_expand`, `memory_page_read`, evidência do preview e `memory_proposal_list`) leem da `release`, com `source: release`, para enxergar o que lotes anteriores já consolidaram. Sem esse parâmetro, `memory_search`, `memory_expand` e `memory_page_read` leem a `main`, e `memory_proposal_list` lê a `proposals`. A busca do dia a dia nunca lê uma `release` ainda não revisada, e o `/remember` continua vendo as propostas novas durante um release.
- Agrupa as propostas por assunto ou página, e processa em lotes, cada um sendo um commit na `release`.
- Cada proposta é consumida em memória canônica ou descartada com motivo (`not_relevant`, `incorrect`, `sensitive`, `wrong_target`, `superseded`, `duplicate`). O commit que altera a memória remove a proposta.
- Não existe `keep_pending` nem forma de adiar: todas as propostas do marcador entram no release, e ele só termina com `proposals/` vazia na `release`. Cada lote tem no máximo 16 propostas.
- Preview do diff e confirmação explícita antes de cada aplicação. Cada lote é aplicado por uma transação com marcador e recuperação, que avança a `release`.

### 3. Revisar e mergear

- O envio para revisão só é aceito com `proposals/` vazia na `release`.
- Até aqui a `release` existe só na máquina de quem abriu o release. O GPM faz push dela, uma única vez, e abre o PR contra a `main` com `gh pr create`. Antes de agir, procura o PR mais recente dessa branch (em qualquer estado) e decide pelo estado dele:

  | Último PR da branch | Ação |
  |---|---|
  | Aberto | Usa esse PR |
  | Mergeado | Não cria nada; finaliza o release |
  | Fechado sem merge, ou nenhum | Cria um PR novo |

  O marcador não guarda o PR, então uma queda entre criar o PR e qualquer gravação não deixa estado a reparar. Revisão e merge são humanos (de preferência com squash).
- Novos commits na `release` depois de aberto o PR entram no mesmo PR. Como são feitos (por exemplo, pela interface do GitHub) fica fora do GPM.

### 4. Finalizar o release

Roda quando o PR do release está mergeado. O PR é procurado pela branch do release, em qualquer estado, e a ação segue a tabela de "Revisar e mergear". Isso vale para qualquer tipo de merge (squash, merge comum ou rebase), não depende de a `release` existir e ignora edições manuais na `main`. Antes de agir, o GPM confirma que o commit de merge do PR já está em `origin/main`. Começa com `fetch --prune`, `merge origin/proposals` (para não ser rejeitado por propostas novas) e então, em um único push na `proposals`, ele:

1. faz merge de `origin/main`;
2. remove as propostas listadas no marcador (as que o merge comum já apagou são ignoradas);
3. remove o marcador.

O lock cai no mesmo instante. Depois, apaga a `release` e o worktree, se ainda existirem. Isso não faz parte do lock.

Se o PR foi fechado sem merge, o GPM não age sozinho: informa a situação e a pessoa escolhe entre `--resume` (cria outro PR da mesma `release`) e `--abort`.

Quando roda:

- De forma síncrona no início do `/promote`, antes de abrir um novo release. Se falhar, o comando informa o erro e não abre o release.
- Sob demanda, com `gpm promote --finish`. Pede confirmação de que o PR foi mergeado e serve quando o `gh` não está disponível ou o PR foi mergeado por outro caminho.

Não há processo em segundo plano: a finalização só roda dentro de um comando, sob o lock do repositório. O `/remember` não finaliza nada. A memória da `proposals` só é atualizada pelo merge de `origin/main` no `Open` e na finalização. Isso basta, porque ninguém lê a memória da `proposals`: a leitura vem da `main`.

A finalização é idempotente: se for interrompida no meio, qualquer nova execução converge. Um atraso nela não prejudica a leitura (que vem da `main`) nem o `/remember` (que só escreve em `proposals/`), e o marcador impede um novo release até a finalização.

### Release abandonado

- `gpm promote --resume` continua o release em andamento. As propostas pendentes são as que ainda estão em `proposals/` na `release`. Se o PR foi fechado sem merge, cria outro. De onde ele lê a `release`:
  - se existe localmente, continua dela (caso normal, na mesma máquina);
  - senão, se existe `origin/release/<opened_at>` (só depois do envio para revisão), faz checkout dela, e é o único caso em que outra máquina consegue retomar;
  - senão, não há o que retomar: só `--abort`. O GPM não recorta de `base`, porque sem a branch não dá para saber se havia lotes aplicados.
- `gpm promote --abort` recusa se o PR do release já está mergeado (orienta a usar `--finish`). Senão, remove o marcador e apaga a `release`, se existir. As propostas permanecem na `proposals`, intactas. Antes do envio para revisão, o trabalho de consolidação existe só na máquina de quem abriu o release: se ela for perdida, outra pessoa só consegue `--abort` e refazer a consolidação, sem perder conhecimento.

## Questões em aberto

- Tamanho máximo de um release quando o backlog é grande.
