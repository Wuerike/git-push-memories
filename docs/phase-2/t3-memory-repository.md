# T3: Memory Repository

## Fixed point e interface

- Fixed point: `0b1e30283f23bae811dc733ac8d38304026007bf`.
- `Repository.Inspect(ctx, Scope)` resolve um arquivo Markdown e devolve seu snapshot atual.
- `Repository.Apply(ctx, ChangeSet)` substitui o corpo de uma secao folha se o hash do arquivo ainda corresponder ao baseline.
- `Repository.Close()` libera o descritor que fixa a identidade do memory root durante a vida do modulo.

O snapshot expoe somente path canonico, secoes, hash SHA-256 dos bytes reais, HEAD e blob Git. O change set transporta target, baseline e replacement. Parser, offsets, paths absolutos, locks, temporarios, comandos Git e fault injection ficam privados ao modulo.

## Invariantes

- O memory root ja validado e a unica raiz acessivel ao modulo; paths absolutos, qualquer segmento `..`, arquivos nao Markdown e symlinks falham.
- Toda leitura e escrita valida o schema Markdown v1.
- Apply revalida path, schema e baseline sob lock entre processos.
- A escrita usa temporario adjacente, `fsync`, rename e `fsync` do diretorio.
- O modulo nao recebe code root e nao executa mutacoes Git.

## Contratos concretos

`Snapshot` contem o path relativo canonico, secoes com ID/heading/body/hash/leaf, hash SHA-256 do arquivo completo e os IDs Git de HEAD e do blob do target. `Receipt` contem o target canonico e os hashes de baseline e post-state.

Os erros sentinela sao `ErrInvalidPath`, `ErrInvalidSchema`, `ErrSectionNotFound`, `ErrSectionNotLeaf` e `ErrDrift`. Uma divergencia do hash real e reportada antes do parse, portanto qualquer byte concorrente produz drift mesmo quando torna o arquivo invalido.

O lock usa `flock` no descritor do diretorio raiz, sem criar estado no worktree. O descritor e aberto uma vez por `New`, permanece fixo mesmo se o pathname da raiz for trocado e tambem e herdado pelos subprocessos Git. Leituras usam lock compartilhado e apply usa lock exclusivo; um mutex serializa chamadas da mesma instancia porque `flock` pertence ao descritor compartilhado. Arquivos e diretorios de escrita sao abertos por descritores com `openat2`, `RESOLVE_BENEATH` e `RESOLVE_NO_SYMLINKS`, eliminando troca de symlink entre validacao e uso.

A seam privada de escrita injeta falhas depois de create, write, `fsync` e rename. Falhas anteriores ao rename preservam o baseline e removem o temporario; rename e o ponto de commit descrito pela ADR. Na inicializacao, nomes temporarios controlados e incompletos sao removidos sob o mesmo lock. A implementacao de producao preserva o modo do arquivo, grava um temporario adjacente, sincroniza, renomeia e sincroniza o diretorio.

Frontmatter delimitado por `---` ou `...` e mascarado somente durante o parse, preservando offsets e bytes sem participar da identidade. Marcadores de conflito sao rejeitados em linhas estruturais, mas exemplos inline ou dentro de code fences permanecem validos. O blob e sempre consultado contra o ID de HEAD capturado pelo snapshot, nunca contra uma segunda leitura simbolica de `HEAD`.

Os testes descartaveis tambem inventariam code root, indice e objetos Git antes/depois das operacoes. Somente o Markdown alvo muda e nenhum temporario permanece apos sucesso.

## Verificacao

O filtro focado e `go test ./internal/memory`. A matriz final e `make verify` seguida de `make verify-offline`.
