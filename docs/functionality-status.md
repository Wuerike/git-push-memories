# Estado das funcionalidades

Descreve o que a implementacao atual expoe ao mundo externo e o que cada acao altera.

## Modelo em uma tela

- Um **repositorio de memoria** (Git, branch `main`, `gpm.json` na raiz) guarda dois namespaces:
  - `memories/<categoria>/**/*.md` — memoria canonica, unica coisa pesquisavel;
  - `proposals/<id>-<slug>.md` — inbox de propostas, fora da busca.
- Toda escrita e um **commit local em `main`**. O produto nunca faz fetch, pull, push nem abre PR; enviar e atualizar entre clones e Git comum, feito pela pessoa.
- Nao ha daemon, porta, banco ou hook de sessao. O MCP roda por stdio, iniciado pelo Claude Code.

## 1. Instalacao e setup

| Acao | Efeito |
|---|---|
| `install.sh` / `make install` | Instala o binario `gpm` (padrao `~/.local/bin`). |
| `gpm setup <memory-root>` | Valida o repositorio (worktree Git com ao menos um commit e `gpm.json` valido). Exige `claude` no `PATH`. Grava o memory root em `<UserConfigDir>/git-push-memories/config.json`, registra `gpm mcp` no escopo `user` do Claude Code e instala `~/.claude/skills/{remember,promote}/SKILL.md`. Recusa sobrescrever skill com mesmo nome que nao seja gerenciado por este projeto. Pode ser repetido para trocar de repositorio ou apos mover o binario. |

`gpm.json`:

```json
{ "schema_version": 1, "memory_categories": ["domains", "decision"] }
```

Categorias: lista nao vazia, sem duplicatas, lowercase kebab-case. Campos desconhecidos ou conteudo extra invalidam o arquivo.

Locks e marcadores de recovery ficam em `$XDG_STATE_HOME/git-push-memories` (ou `~/.local/state/...`), fora de qualquer worktree.

## 2. CLI

| Comando | Efeito |
|---|---|
| `gpm setup <memory-root>` | Ver acima. |
| `gpm retrieve <query> --budget-bytes <n> [--path-prefix <p>]` | Busca textual na memoria canonica; imprime JSON na saida padrao. Somente leitura. `--path-prefix` restringe a uma categoria, subdiretorio ou arquivo. |
| `gpm mcp` | Servidor MCP por stdio (usado pelo Claude Code, nao manualmente). Ao iniciar, conclui ou desfaz uma curadoria interrompida. |
| `gpm version`, `gpm help` | Funcionam sem configuracao. |

## 3. Comandos no Claude Code

Ambos so rodam quando invocados pela pessoa (nao sao acionados pelo modelo) e exigem confirmacao explicita antes de qualquer escrita.

| Comando | O que acontece | Efeito no repositorio |
|---|---|---|
| `/remember <fato>` | Interpreta o fato (assunto, escopo, forma), pesquisa a memoria e as propostas pendentes, avisa se ja existe proposta do mesmo assunto, sugere `create`, `update` ou `remove` e mostra os campos exatos. Apos confirmar, chama `memory_propose` uma vez. | **1 commit** em `main` que adiciona `proposals/<id>-<slug>.md`. Memoria canonica nao muda. Proposta identica a uma existente reaproveita a existente (`duplicate: true`), sem commit. Falha ou cancelamento nao escrevem nada. |
| `/promote [id ...]` | Lista o inbox (ou usa os IDs dados, de 1 a 16), agrupa por assunto/pagina com aprovacao, mostra plano, diff completo e evidencias; repete previews ate a pessoa confirmar o diff exato. | **1 commit** em `main` que aplica todas as decisoes do lote e remove as propostas consumidas do inbox. Cancelar nao altera nada. |

Decisoes possiveis em `/promote`:

| Decisao | Efeito |
|---|---|
| `create_page`, `create_section` | Cria pagina nova ou acrescenta secao H2 a pagina existente. |
| `replace_section` | Troca o corpo de uma secao folha. |
| `rewrite_page` | Reescreve a pagina inteira (IDs de headings novos gerados pelo servidor; frontmatter preservado; nunca apaga a pagina). |
| `remove_section`, `remove_page` | Remove secao folha; pagina so se for a forma gerada com uma unica secao `Overview`. Sem cascata nem reparo de links. |
| `fold_proposal` | Absorve proposta extra em um `rewrite_page` ou `create_page` com pagina completa do mesmo lote. Sem mudanca canonica propria. |
| `discard_proposal` | Remove so a proposta e registra motivo fechado (`not_relevant`, `incorrect`, `sensitive`, `wrong_target`, `superseded`, `duplicate`) no commit. |
| `keep_pending` | Deixa a proposta intacta no inbox; exige outra decisao no lote que gere commit. |

Regras do lote: targets canonicos distintos (exceto folds), preview de uso unico, e qualquer mudanca de `HEAD`, conteudo ou integridade de referencia invalida o preview.

## 4. Ferramentas MCP

| Tool | Le/escreve | Efeito |
|---|---|---|
| `memory_search` | le | Ranking textual de secoes folha da memoria canonica dentro de `budget_bytes`; devolve handles. |
| `memory_expand` | le | Corpo de uma secao por handle; rejeita handle obsoleto. |
| `memory_page_read` | le | Pagina inteira com `content_hash`; `budget_too_small` em vez de truncar. |
| `memory_proposal_list` | le | Propostas do inbox (revalidadas) e categorias configuradas. |
| `memory_reference_snapshot` | le | Referencias imutaveis (lineage, commit, path, SHA-256 do blob) para ate 16 arquivos de um checkout local de codigo; o root local e vinculado so ao processo atual e nunca persistido. |
| `memory_reference_expand` | le | Trecho de linhas de uma referencia `verified` (UTF-8, ate 64 KiB). |
| `memory_propose` | **escreve** | Commit da proposta no inbox (ver `/remember`). |
| `memory_promote_preview`, `memory_promote_batch_preview` | le | Diff e evidencias de 1 a 16 decisoes, mais token de uso unico. Novo preview invalida tokens anteriores das mesmas propostas. |
| `memory_promote_apply`, `memory_promote_batch_apply` | **escreve** | Consome o token e cria o commit da promocao. |

## 5. Restricoes que o usuario sente

- **Branch**: `memory_propose`, `memory_proposal_list` e toda curadoria exigem o checkout do repositorio de memoria exatamente em `main` (erro `not_on_main`). Busca e leitura nao exigem.
- **Staging alheio preservado**: commits usam plumbing Git (`commit-tree` + `update-ref` com compare-and-swap), tocam so os paths necessarios e **nao executam hooks de commit**. Com `index.lock` ativo, a operacao falha (`git_index_locked`).
- **Concorrencia**: locks serializam sessoes no mesmo clone. Entre clones, conflito (push non-fast-forward) e resolvido com Git.
- **Recovery**: curadoria interrompida converge para o estado anterior ao `update-ref` ou para o commit completo depois dele; lock de outro processo nunca e removido.
- **Privacidade (fail-closed)**: o payload da proposta e validado antes de qualquer lock, arquivo ou objeto Git. Bloqueia chaves, tokens, autorizacao, e-mail, telefone, CPF e referencias de sessao. Texto livre que nao case com esses padroes nao e classificado semanticamente.
- **Fontes**: `source_paths` (v1) aparecem sempre como `legacy_unverified`; `source_references` (v2) sao `verified` somente com o checkout local vinculado, senao `unavailable` ou `mismatch`. Integridade nao prova que a fonte sustenta o fato; isso e avaliado a parte (`supports`/`contradicts`/`unclear`) pela skill e pela pessoa.

## 6. O que nao existe

- Captura automatica por hook de sessao.
- Fetch, pull, push, PR ou politica de merge.
- Busca semantica, vetorial ou indice persistente.
- Deteccao semantica completa de duplicidade ou contradicao (so dedupe byte a byte e evidencia assistida).
- Autoridade de curadoria para agentes.
- `gpm proposal ...` e tools `memory_propose_create_*`/`proposal_apply|publish` (lifecycle antigo, removido).

## 7. Verificacao

```bash
make verify          # go test, go test -race, go vet, go build
make verify-offline  # idem, sem proxy de modulos
tests/e2e/run-phase-3.sh  # E2E com binario real em repositorios descartaveis
```
