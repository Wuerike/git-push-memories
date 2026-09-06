# T4: recuperacao textual

## Fixed point e seams

O fixed point de T4 e `1c6a8b82a63f9e204adbbbf7cb0f5f5ac974068b`. O modulo fica sob `internal/retrieval`, consome somente paths canonicos e snapshots de `Memory Repository` e expoe `Retrieve` e `Expand`. O adapter MCP traduz somente `memory_search` e `memory_expand`; parser, offsets, filesystem interno e Git nao atravessam essas interfaces.

Os filtros focados sao `go test ./internal/retrieval`, `go test ./internal/mcpserver -run 'TestMemory(Search|Expand)'` e `go test ./internal/app -run TestMCP`. A matriz final e `make verify` seguida de `make verify-offline`.

## Contrato deterministico

- A consulta e normalizada para minusculas e tokens Unicode de letras ou numeros. Tokens repetidos contam uma vez e consulta sem tokens e invalida.
- Somente secoes folha participam do ranking, evitando repetir no pai o conteudo dos filhos.
- O score e a quantidade de tokens distintos da consulta presentes no path, heading ou corpo. Resultados sem match nao entram.
- Score maior vem primeiro. Empates usam path canonico, ordem da secao no documento e section ID.
- O excerpt de busca e o corpo completo da secao sem espaco nas extremidades. `Expand` retorna o corpo integral, inclusive seu newline final.
- O handle serializavel contem path, section ID e hash da secao. Alterar essa secao torna o handle obsoleto; alterar outra secao nao.

## Scan e budget

O scan nao cria indice e considera arquivos regulares `.md` sob o memory root. Nao segue symlinks e exclui `.git`, `.git-push-memories`, `proposals`, `receipts`, arquivos nao Markdown e temporarios controlados `.git-push-memories-<16 hex>.tmp`.

Um Markdown incluido que nao possa produzir snapshot autoritativo faz a busca falhar; o modulo nao ignora silenciosamente arquivo invalido, ilegivel ou sem blob no HEAD.

O budget mede `json.Marshal(ContextBundle)` em bytes UTF-8, antes do envelope MCP. Busca inclui o maior prefixo completo do ranking que cabe e sinaliza `truncated`; nunca corta JSON, item ou string. Se nenhum match completo couber, retorna `ErrBudgetTooSmall`. Expand exige que o item inteiro caiba e retorna o mesmo erro tipado caso contrario.

Retrieval abre o Memory Repository em modo read-only: inicializacao, busca, expansao e encerramento nao removem temporarios nem gravam filesystem ou Git.

A CLI read-only usa `retrieve <query>` com `--memory-root`, `--code-root`, `--budget-bytes` e `--path-prefix` opcional. As duas roots continuam explicitas mesmo que a operacao leia somente memoria. No MCP, falha de configuracao ou transporte termina o processo sem retry, spool ou hook; a prova de que Claude Code segue a sessao sem o servidor pertence ao harness de integracao de T8.
