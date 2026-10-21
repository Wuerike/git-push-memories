# T11: reestruturacao de paginas na curadoria

Status: **implementada e verificada por testes automatizados em 2026-10-20; protocolo de aceitacao manual escrito e NAO executado pelo implementador.**

## Hipotese

Quando duas ou mais propostas descrevem o mesmo assunto, o `/curate` consegue consolidar tudo em uma unica pagina coerente, com titulo e estrutura do assunto (por exemplo, um time com lista de membros), em um unico commit e sem paginas intermediarias, sempre sob revisao do diff exato e iteracao com o usuario.

## Ponto fixo e seam

- Ponto fixo da implementacao: `5d136a8`.
- Seam publica: skill `/curate [proposal-id ...]` com MCP sobre stdio.
- Operacoes adicionadas: `rewrite_page`, `fold_proposal` e `create_page` com `replacement`; tool `memory_page_read`.
- Decisoes: [ADR 0007](../adr/0007-page-rewrite-and-folded-proposals.md), A1-A10.
- Origem: experimento de 2026-10-20 em que cada proposta gerava uma secao e a pagina final tinha o nome de uma pessoa como titulo.

## Cenario

Dois prompts originais, usados textualmente com o `/remember` atual em um repositorio de memoria descartavel, sem nenhuma pagina previa:

```text
/remember meu nome é wuerike e eu so do time da camada de leitura
/remember o time de camada de leitura tem outro membro que se chama guilherme
```

Fluxo esperado do `/curate`:

```text
memory_proposal_list
-> propor o grupo das duas propostas ("pagina nova") e aguardar aprovacao
-> plano da pagina com ledger de fatos
-> UM memory_curate_batch_preview: create_page + replacement (proposta dona)
   e fold_proposal (fold_into = proposta dona)
-> diff completo no corpo da mensagem
-> pedido de ajuste ("troque o titulo para ...") gera novo preview na mesma execucao
-> confirmacao do ultimo diff
-> memory_curate_batch_apply com o token da ultima rodada
-> um unico commit; proposals/ vazio
```

## Contratos entregues

- `memory_page_read` devolve a pagina inteira e o `content_hash`, ou `budget_too_small`; nunca uma pagina truncada.
- `rewrite_page` substitui a pagina existente inteira, vinculada ao hash observado; o servidor gera IDs dos headings novos e preserva o frontmatter do baseline; nunca apaga a pagina.
- `fold_proposal` (somente em lote) consome uma proposta extra sem mudanca canonica propria; `fold_into` aponta para um `rewrite_page` ou um `create_page` com `replacement` do mesmo lote.
- `create_page` aceita `replacement` com a pagina completa, o que cria uma pagina nova de duas ou mais propostas em uma unica execucao.
- Um novo preview para uma proposta invalida tokens anteriores que a contenham (A8).
- A skill `/curate` agrupa o inbox com aprovacao, produz plano e ledger por pagina, mostra o diff no corpo da mensagem e itera.
- Testes automatizados: `internal/mcpserver/rewrite_fold_test.go` (servidor em processo) e `TestE2EReadingTeamPageIsRestructuredByRewrite` / `TestE2ENewTeamPageIsCreatedFromTwoProposalsInOneRun` em `internal/mcpserver/claude_harness_test.go` (binario real e processo MCP novo para a recuperacao).

## Protocolo manual

Executar com Claude Code e um repositorio de memoria descartavel (`git init`, `gpm.json` valido, ao menos um commit, branch `main`), configurado com `gpm setup`. Gravar o transcript em `.local/` (nao versionado).

1. Criar as duas propostas com os prompts originais acima.
2. Rodar `/curate` e conduzir o fluxo esperado, pedindo um ajuste de titulo antes de confirmar.
3. Conferir o repositorio: `git log --stat`, a pagina criada e `proposals/`.
4. Repetir os cenarios extras S1-S4.

### Rubrica

| Item | Criterio |
|---|---|
| R1 | `/curate` propoe o grupo das duas propostas antes de qualquer pagina e aguarda aprovacao |
| R2 | O titulo da pagina final e do time, nao de uma pessoa |
| R3 | Os membros aparecem numa lista unica, sem uma secao por pessoa |
| R4 | Ha um unico commit de curadoria e `proposals/` fica vazio |
| R5 | Nenhum fato que o usuario nao disse (por exemplo, descricao das atribuicoes do time) |
| R6 | O ledger listou o `Overview` antigo como `mesclado` ou `removido` (quando existir pagina previa; sem pagina previa, registrar como nao aplicavel) |
| R7 | O diff apareceu no corpo da mensagem antes da confirmacao, nao so no resultado da ferramenta |
| R8 | Pedir um ajuste (por exemplo, "troque o titulo para X") gera um novo diff na mesma execucao, e so a confirmacao da ultima rodada aplica; o commit reflete o ajuste |
| R9 | A pagina foi criada em uma unica execucao: nenhuma pagina intermediaria com uma pessoa como titulo |

### Cenarios extras

- S1: tres propostas em dois assuntos geram dois grupos; aprovar so um deixa o outro intacto.
- S2: pagina ja consolidada e depois uma proposta com um terceiro membro: a curadoria faz `rewrite_page` sobre a pagina consolidada e a lista passa a ter tres nomes.
- S3: curar entre os dois `/remember` (ordem inversa): a segunda curadoria reescreve a pagina criada pela primeira e termina com lista de membros.
- S4: duas propostas sobre uma pagina existente cujo titulo esta errado (fixture da T3): a curadoria faz `rewrite_page` mais `fold_proposal` e corrige o titulo.

Registrar passou/falhou por item e por cenario.

## Resultado

a executar

O implementador nao executou este protocolo manual. Nenhum resultado foi inventado; a verificacao feita ate aqui e a automatizada (`make verify`).

## Riscos

- O `/remember` atual pode redigir as propostas com texto diferente do esperado; a rubrica avalia o resultado da curadoria, nao a redacao das propostas.
- Perda de fatos nao e detectada pelo servidor (A3); a protecao e o diff completo mais o ledger sob confirmacao humana.
- O agrupamento depende do julgamento do modelo (A5); grupos ruins sao corrigidos pelo usuario na aprovacao.
- Se R2, R3 ou R9 falharem de forma recorrente, o `/remember` interpretativo do Plano B (propostas atualizaveis) passa a ser a alternativa.
