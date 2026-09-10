# T4: remocao de conhecimento e descarte de proposta

Status: **implementada e verificada em 2026-09-10**.

## Hipotese

Uma secao folha ou pagina gerada obsoleta pode ser removida, e uma proposta invalida pode ser descartada, com preview e decisao Git observavel sem criar receipts ou outro store duravel.

## Ponto fixo e seam

- Ponto fixo da implementacao: `ecb20fc`.
- Seam publica: skill `/curate [proposal-id]` com MCP sobre stdio.
- Operacoes canonicas adicionadas: `remove_section`, `remove_page` e `discard_proposal`.
- Identidade de remocao: path, section ID e content hash obtidos de um handle atual.
- Reason codes de descarte: `not_relevant`, `incorrect`, `sensitive`, `wrong_target` e `superseded`.

## Cenarios implementados

```text
proposta remove
-> search + expand
-> remove_section para target folha
   ou remove_page para pagina gerada elegivel
-> preview do target + remocao da proposta
-> confirmacao explicita
-> commit com decisao consumed
```

```text
proposta que nao deve virar memoria
-> search + expand para sustentar a decisao
-> discard_proposal com reason code fechado
-> preview somente da remocao da proposta
-> confirmacao explicita
-> commit com decisao discarded + reason
```

## Contratos entregues

### Remocao canonica

- `remove_section` aceita somente proposta `remove` e secao folha que nao seja o H1.
- O splice remove exatamente o intervalo da secao; headings, anchors e bytes restantes conservam ordem e conteudo.
- `remove_page` aceita somente a forma byte-exata gerada por `create_page`: H1 derivado do path, sem corpo proprio, seguido por uma unica secao `Overview` com ID derivado.
- Pagina com fatos no H1, secoes adicionais ou forma manual deve ser reduzida explicitamente por secoes e nao e apagada como unidade.
- Nao ha cascade delete nem reparo automatico de links.

### Descarte

- `discard_proposal` pode encerrar proposta `create`, `update` ou `remove` sem target canonico.
- O preview exige exatamente um reason code do conjunto fechado.
- Campos de target, heading, replacement e content hash sao rejeitados no descarte; `reason` e rejeitado nas operacoes de consumo.
- Cancelar depois do preview nao remove a proposta; somente um apply confirmado consome o token single-use.

### Apply e Git

- Toda operacao recalcula o preview e revalida configuracao, `HEAD`, proposta, privacidade e identidade antes de publicar.
- Remocao de secao cria exatamente `M <pagina>` e `D <proposta>`.
- Remocao de pagina cria exatamente `D <pagina>` e `D <proposta>`.
- Descarte cria exatamente `D <proposta>` e nao altera memoria canonica.
- Commits de consumo registram `Git-Push-Memories-Decision: consumed`.
- Commits de descarte registram `Git-Push-Memories-Decision: discarded` e `Git-Push-Memories-Reason: <reason>`.
- Staging e worktree alheios permanecem intactos; rollback de pagina preserva os bytes e o modo original do worktree.

## Testes e evidencias

Os testes focados cobrem:

- remocao byte-preserving de secao folha e ausencia posterior na recuperacao;
- rejeicao de H1 nao folha e content hash obsoleto sem escrita;
- remocao de pagina gerada elegivel e ausencia posterior na recuperacao;
- rejeicao de pagina rica e de conhecimento proprio no H1;
- drift entre preview e apply sem consumo da proposta, com token single-use;
- descarte sem mudanca de memoria e com preservacao de staging e worktree alheios;
- reason code invalido e cancelamento de preview sem remocao;
- paths exatos e trailers distintos para consumo e descarte;
- regressao das operacoes `create_page`, `create_section` e `replace_section`.

Comandos focados principais:

```text
go test ./internal/memory ./internal/mcpserver ./tests/claudeadapter -count=1
```

Verificacao final:

```text
make verify
```

Resultado em 2026-09-10: `go test ./...`, `go test -race ./...`, `go vet ./...` e `go build ./...` passaram com Go 1.25.12.

## Limites mantidos

- A proposta continua sugerindo somente um path; a identidade precisa e escolhida na curadoria.
- Paginas manuais ou com mais de uma secao nao sao elegiveis para remocao integral.
- Nao ha cascade delete, verificacao ou reparo de links.
- Duplicidade e contradicao ainda dependem da classificacao humana sobre candidatos limitados; garantias adicionais pertencem a T5.
- Lotes e recovery posterior ao `update-ref` permanecem para T7.

## Criterio de conclusao

T4 esta concluida porque os tres destinos possuem preview sem escrita, confirmacao e commit exato; conhecimento removido deixa de ser recuperado; descarte nao altera memoria; Git distingue as decisoes; e falha, drift ou cancelamento preservam a proposta.
