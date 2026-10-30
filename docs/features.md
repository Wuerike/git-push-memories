# Funcionalidades

Descreve o que a implementacao atual expoe ao mundo externo e o que cada acao altera, e lista o que ainda e parcial ou nao foi iniciado. O fluxo completo esta em [workflow.md](workflow.md).

## Modelo em uma tela

- Um **repositorio de memoria** remoto (GitHub para o `/promote`) com `gpm.json` na raiz e tres branches: `main` (memoria canonica publicada, protegida), `proposals` (main + propostas pendentes) e `release/<opened_at>` (consolidacao, vira PR para a `main`).
- O GPM mantem um **clone bare** em `$XDG_STATE_HOME/git-push-memories/<repo-id>` e worktrees `read` (`main`), `proposals` e `release`. Nunca toca em repositorios da pessoa.
- `memories/<categoria>/**/*.md` e a unica coisa pesquisavel; `proposals/<id>-<slug>.md` e o inbox, fora da busca.
- Nao ha daemon, porta, banco ou hook de sessao. O MCP roda por stdio, iniciado pelo Claude Code.

## 1. Instalacao e setup

| Acao | Efeito |
|---|---|
| `install.sh` / `make install` | Instala o binario `gpm` (padrao `~/.local/bin`). |
| `gpm setup <url> [--categories a,b] [--yes]` | Idempotente. Clona (bare) a URL; remote vazio: cria `gpm.json`, `main` e `proposals` e envia; remote com `gpm.json`: valida e usa a `proposals` existente (ou a cria); remote com conteudo sem `gpm.json`: erro. Cria os worktrees `read` e `proposals`, registra `gpm mcp` no escopo `user` do Claude Code, instala `~/.claude/skills/{remember,promote}/SKILL.md` e grava `<UserConfigDir>/git-push-memories/config.json` (`repo_url`). Exige `git`, `claude` e identidade Git. URL diferente da configurada e erro. |

`gpm.json`:

```json
{ "schema_version": 1, "memory_categories": ["domains", "decision"] }
```

Categorias: lista nao vazia, sem duplicatas, lowercase kebab-case. Campos desconhecidos ou conteudo extra invalidam o arquivo.

## 2. CLI

| Comando | Efeito |
|---|---|
| `gpm setup <url>` | Ver acima. |
| `gpm retrieve <query> --budget-bytes <n> [--path-prefix <p>]` | Busca textual na `main` publicada (atualiza o worktree `read` se o TTL de 10 min venceu); imprime JSON. |
| `gpm promote --resume` | Retoma o release local; sem propostas pendentes, envia para revisao (push + PR). |
| `gpm promote --abort` | Remove o marcador e a `release`; propostas intactas. Recusa se o PR ja foi mergeado. |
| `gpm promote --finish` | Finaliza um release com PR mergeado (confirma manualmente se o `gh` nao estiver disponivel). |
| `gpm mcp` | Servidor MCP por stdio. Ao iniciar, conclui ou desfaz uma promocao interrompida. |
| `gpm version`, `gpm help` | Funcionam sem configuracao. |

## 3. Comandos no Claude Code

Ambos so rodam quando invocados pela pessoa e exigem confirmacao explicita antes de qualquer escrita.

| Comando | O que acontece | Efeito |
|---|---|---|
| `/remember <fato>` | Interpreta o fato, pesquisa a memoria e as propostas pendentes para sugerir alvo e acao (`create`, `update` ou `remove`) e avisar de propostas ja pendentes sobre o mesmo assunto, e mostra os campos exatos. A busca e orientacao, nao deteccao de duplicidade ou contradicao. Apos confirmar, chama `memory_propose`. | Um commit na `proposals` que adiciona `proposals/<id>-<slug>.md`, criado direto no remote pela API do GitHub, em uma chamada. Se falhar, nada e gravado e a pessoa repete o `/remember`. A memoria canonica nao muda. |
| `/promote` | Abre (ou retoma) o release, agrupa **todas** as propostas por assunto/pagina, classifica cada candidato canonico, fato da pagina e proposta relacionada como `equivalent`, `contradictory`, `related` ou `unrelated` e as consolida em lotes de ate 16, com plano, diff e evidencias; ao zerar, envia para revisao. | Um commit por lote na `release` (memoria + remocao das propostas); push da branch e PR para a `main`. Merge e revisao sao humanos (squash recomendado). |

Decisoes possiveis em `/promote`:

| Decisao | Efeito |
|---|---|
| `create_page`, `create_section` | Cria pagina nova ou acrescenta secao H2 a pagina existente. |
| `replace_section` | Troca o corpo de uma secao folha. |
| `rewrite_page` | Reescreve a pagina inteira (IDs de headings novos gerados pelo servidor; frontmatter preservado; nunca apaga a pagina). |
| `remove_section`, `remove_page` | Remove secao folha; pagina so se for a forma gerada com uma unica secao `Overview`. Sem cascata. |
| `fold_proposal` | Absorve proposta extra em um `rewrite_page` ou `create_page` com pagina completa do mesmo lote. |
| `discard_proposal` | Remove so a proposta e registra motivo fechado (`not_relevant`, `incorrect`, `sensitive`, `wrong_target`, `superseded`, `duplicate`). |

Duplicidade e contradicao sao tratadas aqui, por decisao de design: a consolidacao e o unico ponto em que todas as propostas e a pagina canonica completa aparecem juntas. O agente classifica e resolve com `fold_proposal` ou `discard_proposal` (`duplicate`, `superseded`); a pessoa confirma cada lote e revisa o PR. O inbox pode conter propostas repetidas ou conflitantes sem dano, pois nada nele e memoria canonica.

Nao ha como adiar uma proposta: o release so termina com `proposals/` vazia. Regras do lote: targets canonicos distintos (exceto folds), preview de uso unico, e qualquer mudanca de `HEAD` ou conteudo invalida o preview.

## 4. Ferramentas MCP

| Tool | Le/escreve | Efeito |
|---|---|---|
| `memory_search`, `memory_expand`, `memory_page_read` | le | Busca, expansao por handle e pagina inteira com `content_hash`. `source`: `main` (padrao) ou `release`. |
| `memory_proposal_list` | le | Propostas (texto livre; arquivos ilegiveis sao ignorados) e categorias. `source`: `proposals` (padrao) ou `release`. |
| `memory_propose` | **escreve** | `title` e `content` livres; commit da proposta na `proposals` remota via API do GitHub (`gh api`), tudo ou nada. Erros: `invalid_proposal`, `gh_unavailable`, `push_denied`, `proposal_failed`. |
| `memory_promote_open`, `memory_promote_status` | **escreve** / le | Abre (marcador enviado = lock) ou retoma o release; mostra pendentes. |
| `memory_promote_preview`, `memory_promote_batch_preview` | le | Diff e evidencias de 1 a 16 decisoes na `release`, com token de uso unico. |
| `memory_promote_apply`, `memory_promote_batch_apply` | **escreve** | Consome o token e cria o commit do lote na `release`. |
| `memory_promote_submit` | **escreve** | Push da `release` e PR via `gh` (ou reaproveita/finaliza conforme o ultimo PR). |

## 5. Restricoes que o usuario sente

- **Worktrees gerenciados**: o GPM so escreve nos seus worktrees (o `/remember` nao usa worktree); se um deles for alterado por fora o `/promote` falha (`wrong_branch`, `worktree_dirty`). Commits ignoram hooks, assinatura e templates da configuracao Git da pessoa.
- **Leitura**: sempre da `main` publicada; uma mudanca pode levar ate 10 min (mais uma leitura) para aparecer, exceto apos `/promote` ou `--finish`, que atualizam na hora.
- **Rede**: toda chamada tem timeout. Falha de rede nao perde dados no `/remember`; permissao negada e erro.
- **Concorrencia**: locks serializam sessoes na mesma maquina; entre maquinas, o marcador `release.lock.json` enviado a `proposals` e o lock do release.
- **Recovery**: uma promocao interrompida converge para o estado anterior ao avanco da `release` ou para o commit completo; lock de outro processo nunca e removido. Falha de recovery desabilita so o `/promote`.
- **Privacidade**: o codigo nao filtra o conteudo da proposta (sem guard de segredos/PII). A barreira e a skill `remember` e a confirmacao humana no `/remember` e no `/promote`. Uma proposta enviada fica no historico da `proposals` mesmo se descartada depois.
- **Duplicidade e contradicao**: a classificacao e feita pelo agente seguindo a skill, sem verificacao em codigo; a qualidade depende do agente e da revisao humana (confirmacao de cada lote e PR).
- **Fontes**: a proposta nao carrega fonte estruturada; se a origem importa, a skill `remember` a escreve no texto.
- **GitHub**: o `/remember` e o `/promote` exigem `gh` instalado e autenticado (com escrita no repositorio) e rede; todos os comandos levam `-R owner/repo`.

## 6. Parcial

Existe, mas com limite conhecido.

- **Suporte das fontes**: a integridade da referencia e deterministica; se a fonte sustenta o fato (`supports`, `contradicts`, `unclear`) e julgamento humano.
- **Validacao por agentes**: o `/promote` guia o agente com preview e evidencias, mas nao ha criterios automaticos para aceitar, rejeitar ou pedir revisao.
- **Busca**: textual, sem ranking semantico.
- **Release sem dono**: `--resume` e `--abort` valem para qualquer pessoa.
- **Forge**: so GitHub (via `gh`), exigido pelo `/remember` e pelo `/promote`. Nao ha modo offline.
- **Plataformas**: so Linux (`amd64` e `arm64`).
- **Escopo de uso**: um repositorio de memoria por pessoa; validado com um desenvolvedor.

## 7. Nao iniciado

- Captura automatica por hook de sessao.
- Busca semantica, vetorial ou indice persistente (ex.: SQLite).
- Integracoes com Copilot CLI e OpenCode.
- Suporte a macOS, Windows e a outros forges (ex.: GitLab).
- Adiar uma proposta: o release so termina com `proposals/` vazia.
- Tratamento de entrada maliciosa em `proposals/`.
- Correcao do PR aberto pelo gpm.
- Versao minima de `git` e `gh`.
- Metricas de relevancia, correcao e custo de manutencao.

## 8. Verificacao

```bash
make verify          # go test, go test -race, go vet, go build
make verify-offline  # idem, sem proxy de modulos
tests/e2e/run-phase-3.sh  # E2E com binario real em remotos locais, sem rede
```
