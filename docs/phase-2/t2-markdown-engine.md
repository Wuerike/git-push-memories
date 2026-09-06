# T2: motor Markdown adaptado

## Fixed point e seams

- Fixed point: `a103128543c11fdae46d88da38c06d5743737a6d`.
- Parser: `markdown.ParseSections` localiza headings, anchors e offsets sem renderizar novamente o documento.
- Edicao: `markdown.ReplaceSectionContent` substitui somente o corpo delimitado de uma secao.

O pacote fica sob `internal/markdown`. Somente `parse.go`, `splice.go` e `section_id.go` sao adaptados do agent-memory; testes e fixtures sao especificacoes proprias deste produto.

## Comportamento coberto

- Anchors sao reconhecidos somente na linha imediatamente posterior a headings ATX ou Setext.
- IDs duplicados retornam `ErrDuplicateSectionID`; IDs ausentes ou mal posicionados permanecem ausentes.
- Headings dentro de code fences nao criam secoes.
- Offsets permanecem corretos com UTF-8, CRLF, headings adjacentes e arquivo sem newline final.
- `ReplaceSectionContent` preserva heading, anchor e todos os bytes fora do corpo alvo.
- Anchors nao canonicos sao tratados como ausentes; o motor nao insere anchors nem migra paginas legadas.

## Origem e licencas

O inventario distribuido em [`third_party/README.md`](../../third_party/README.md) registra a revisao fixa, os blobs originais, as modificacoes e a closure. Os textos Apache-2.0 do agent-memory e MIT do Goldmark acompanham o inventario. Nenhum outro fonte upstream foi incorporado.

| Fonte adaptado | Blob original | Blob adaptado antes do commit |
|---|---|---|
| `parse.go` | `e4cde2d405d112a67b607f0e0f8a3747b81a76e3` | `547746f90f751b40b626db0a132ab8b7d16d7cb4` |
| `splice.go` | `36c81a926fbd97d89039ffa03d963a1ecc501c12` | `391f53d91be5ae3cdde21e0f26c9f78e10c6f6d0` |
| `section_id.go` | `aa801f90ce4ab852f7771000d9e0f81b648fbff6` | `3182c7eb73d55645156c22a66fc575fd832375e8` |

## Verificacao

O filtro focado de T2 e `go test ./internal/markdown`. A verificacao final usa `make verify` e `make verify-offline`.
