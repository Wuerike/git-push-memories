# Estado das funcionalidades

Descreve o que a implementacao atual expoe ao mundo externo e o que cada acao altera. O fluxo completo esta em [workflow.md](workflow.md).

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
| `/remember <fato>` | Interpreta o fato, pesquisa a memoria e as propostas pendentes, sugere `create`, `update` ou `remove` e mostra os campos exatos. Apos confirmar, chama `memory_propose`. | Um commit na `proposals` que adiciona `proposals/<id>-<slug>.md`, enviado ao remote (`pushed`). Sem rede, fica local e e reenviado no proximo `/remember`. Proposta identica reaproveita a existente. A memoria canonica nao muda. |
| `/promote` | Abre (ou retoma) o release, agrupa **todas** as propostas por assunto/pagina e as consolida em lotes de ate 16, com plano, diff e evidencias; ao zerar, envia para revisao. | Um commit por lote na `release` (memoria + remocao das propostas); push da branch e PR para a `main`. Merge e revisao sao humanos (squash recomendado). |

Decisoes possiveis em `/promote`:

| Decisao | Efeito |
|---|---|
| `create_page`, `create_section` | Cria pagina nova ou acrescenta secao H2 a pagina existente. |
| `replace_section` | Troca o corpo de uma secao folha. |
| `rewrite_page` | Reescreve a pagina inteira (IDs de headings novos gerados pelo servidor; frontmatter preservado; nunca apaga a pagina). |
| `remove_section`, `remove_page` | Remove secao folha; pagina so se for a forma gerada com uma unica secao `Overview`. Sem cascata. |
| `fold_proposal` | Absorve proposta extra em um `rewrite_page` ou `create_page` com pagina completa do mesmo lote. |
| `discard_proposal` | Remove so a proposta e registra motivo fechado (`not_relevant`, `incorrect`, `sensitive`, `wrong_target`, `superseded`, `duplicate`). |

Nao ha como adiar uma proposta: o release so termina com `proposals/` vazia. Regras do lote: targets canonicos distintos (exceto folds), preview de uso unico, e qualquer mudanca de `HEAD`, conteudo ou integridade de referencia invalida o preview.

## 4. Ferramentas MCP

| Tool | Le/escreve | Efeito |
|---|---|---|
| `memory_search`, `memory_expand`, `memory_page_read` | le | Busca, expansao por handle e pagina inteira com `content_hash`. `source`: `main` (padrao) ou `release`. |
| `memory_proposal_list` | le | Propostas revalidadas e categorias. `source`: `proposals` (padrao) ou `release`. |
| `memory_reference_snapshot`, `memory_reference_expand` | le | Referencias imutaveis a blobs de um checkout local de codigo e trechos de linhas. |
| `memory_propose` | **escreve** | Commit da proposta na `proposals` e push (`pushed`, `message`). |
| `memory_promote_open`, `memory_promote_status` | **escreve** / le | Abre (marcador enviado = lock) ou retoma o release; mostra pendentes. |
| `memory_promote_preview`, `memory_promote_batch_preview` | le | Diff e evidencias de 1 a 16 decisoes na `release`, com token de uso unico. |
| `memory_promote_apply`, `memory_promote_batch_apply` | **escreve** | Consome o token e cria o commit do lote na `release`. |
| `memory_promote_submit` | **escreve** | Push da `release` e PR via `gh` (ou reaproveita/finaliza conforme o ultimo PR). |

## 5. Restricoes que o usuario sente

- **Worktrees gerenciados**: o GPM so escreve nos seus worktrees; se um deles for alterado por fora o comando falha (`wrong_branch`, `worktree_dirty`). Commits ignoram hooks, assinatura e templates da configuracao Git da pessoa.
- **Leitura**: sempre da `main` publicada; uma mudanca pode levar ate 10 min (mais uma leitura) para aparecer, exceto apos `/promote` ou `--finish`, que atualizam na hora.
- **Rede**: toda chamada tem timeout. Falha de rede nao perde dados no `/remember`; permissao negada e erro.
- **Concorrencia**: locks serializam sessoes na mesma maquina; entre maquinas, o marcador `release.lock.json` enviado a `proposals` e o lock do release.
- **Recovery**: uma promocao interrompida converge para o estado anterior ao avanco da `release` ou para o commit completo; lock de outro processo nunca e removido. Falha de recovery desabilita so o `/promote`.
- **Privacidade (fail-closed)**: o payload da proposta e validado antes de qualquer arquivo ou objeto Git. Bloqueia chaves, tokens, autorizacao, e-mail, telefone, CPF e referencias de sessao.
- **Fontes**: `source_paths` (v1) aparecem como `legacy_unverified`; `source_references` (v2) sao `verified` somente com o checkout local vinculado. Integridade nao prova que a fonte sustenta o fato.
- **GitHub**: o `/promote` exige `gh` instalado e autenticado; todos os comandos levam `-R owner/repo`.

## 6. O que nao existe

- Captura automatica por hook de sessao.
- Busca semantica, vetorial ou indice persistente.
- Deteccao semantica completa de duplicidade ou contradicao (so dedupe byte a byte e evidencia assistida).
- Proprietario do release: `--resume` e `--abort` valem para qualquer pessoa.
- Tratamento de entrada maliciosa em `proposals/`, correcao do PR aberto pelo gpm e versao minima de `git`/`gh` (adiados; ver `.local/implementation-plan.md`, D9).

## 7. Verificacao

```bash
make verify          # go test, go test -race, go vet, go build
make verify-offline  # idem, sem proxy de modulos
tests/e2e/run-phase-3.sh  # E2E com binario real em remotos locais, sem rede
```
