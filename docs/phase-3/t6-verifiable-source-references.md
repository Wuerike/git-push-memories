# T6: referencias de fonte verificaveis

Status: **implementada e verificada em 2026-09-11**.

## Hipotese

Uma referencia imutavel a um blob Git pode provar localmente sua propria integridade sem acessar remotos, persistir paths da maquina ou afirmar que o conteudo sustenta semanticamente o conhecimento.

## Ponto fixo e seams

- Ponto fixo da implementacao: `6c61eec`.
- Captura: `/remember`, `memory_reference_snapshot`, `memory_reference_expand` e `memory_propose`.
- Revisao: `/curate`, `memory_proposal_list` e evidencia de `memory_curate_preview`.
- Decisao arquitetural: [ADR 0005](../adr/0005-immutable-source-references.md).

## Contrato da referencia

```json
{
  "repository": "git-lineage-sha256:<64 hex>",
  "revision": "<full Git commit object ID>",
  "path": "services/checkout/config.yaml",
  "content_hash": "sha256:<64 hex>"
}
```

- `repository` deriva dos root commits alcancaveis pela revisao, ordenados e resumidos com SHA-256.
- `revision` aceita somente object ID completo de commit, nunca `HEAD`, branch, tag ou hash abreviado.
- `path` e relativo, canonico, UTF-8 e identifica exatamente um blob regular `100644` ou `100755`.
- `content_hash` resume os bytes exatos obtidos com `git cat-file blob`; worktree, filtros e conversoes nao participam.
- O root local e argumento transitorio da ferramenta e nao aparece no retorno, proposta, memoria ou Git.

## Fluxo implementado

```text
memory_reference_snapshot(root, revision?, paths)
-> validar Git root explicito
-> resolver commit e repository lineage
-> exigir blobs regulares
-> calcular hashes dos bytes commitados
-> manter binding local apenas no processo MCP
-> retornar source_references

memory_reference_expand(reference, start_line, budget_bytes)
-> exigir integridade verified no binding atual
-> retornar linhas dos bytes commitados em UTF-8
-> limitar resposta e nunca persistir o excerpt

memory_propose(source_paths=[], source_references=[...])
-> Intake Guard fechado + privacidade
-> proposta Markdown schema v2
-> dedupe byte-exato normal

memory_curate_preview
-> revalidar cada referencia quando o lineage esta vinculado
-> evidence.references com integridade + semantic_support=not_assessed
-> incluir evidencia no hash single-use do preview
```

## Estados de integridade

| Estado | Significado |
|---|---|
| `verified` | binding, lineage, commit, blob, path e hash correspondem localmente |
| `unavailable` | checkout nao foi vinculado ou commit nao existe localmente |
| `mismatch` | identidade, path, tipo de objeto ou hash diverge |
| `legacy_unverified` | proposta v1 possui somente source path, sem identidade suficiente |

Todos os estados retornam `semantic_support: not_assessed`. O GPM nunca converte integridade em `supports`; a skill e a pessoa avaliam separadamente `supports`, `contradicts` ou `unclear`.

## Compatibilidade e autoridade

- Propostas v1 continuam estritamente legiveis e curaveis; seus source paths nunca aparecem como verificados.
- Propostas v2 usam `source_references` e nao podem combinar referencias estruturadas com source paths nao vazios.
- A proposta e seu historico Git preservam a evidencia; paginas canonicas nao recebem metadata de fonte neste incremento.
- Alteracao de status entre preview e apply invalida o token como `curation_drift`.
- Status indisponivel ou divergente e evidencia visivel, nao autorizacao automatica nem bloqueio sem decisao humana.

## Fronteiras de seguranca

- Nenhum fetch, pull, clone ou outro acesso remoto e executado.
- Nenhum arquivo do worktree e lido como evidencia; somente objetos Git commitados.
- Symlinks, trees, submodules, paths ambiguos e revisoes simbolicas sao rejeitados.
- Repositorios shallow sao rejeitados para a identidade de lineage permanecer estavel.
- Variaveis `GIT_*` externas, lazy fetch de partial clone e replace objects nao influenciam snapshot ou verificacao.
- Blobs acima de 8 MiB nao sao carregados; expansao aceita somente UTF-8, usa budget exato e nao persiste conteudo.

## Testes e evidencias

- snapshot identifica lineage, revisao, path e hash do blob;
- worktree sujo nao muda integridade do objeto commitado;
- checkout nao vinculado e revisao ausente retornam `unavailable`;
- hash divergente retorna `mismatch`, nunca `verified`;
- path legado retorna `legacy_unverified`;
- Intake Guard rejeita referencia malformada, duplicada ou combinada com source path;
- proposta v2 renderiza, relê e deduplica referencias estruturadas;
- MCP transporta snapshot para proposta e preview;
- expansao retorna somente o trecho commitado e limitado solicitado;
- restart remove o binding transitorio sem perder a referencia;
- mudanca de status depois do preview invalida apply e preserva proposta.

Verificacao final:

```text
make verify
```

Resultado em 2026-09-11: `go test ./...`, `go test -race ./...`, `go vet ./...` e `go build ./...` passaram com Go 1.25.12.

## Criterio de conclusao

T6 esta concluida porque a origem possui identidade nao ambigua no contrato, bytes commitados sao verificaveis quando o checkout e fornecido, ausencia ou divergencia nunca aparece como verificada, e integridade permanece explicitamente separada de sustentacao semantica.
