# T7: transacao multi-path e recovery

Status: implementada e verificada em 2026-09-11.

## Ponto fixo e seams

- Base fixa: `edf00c4` (`feat: add verifiable source references`).
- Motor: `Curator.PreviewBatch`, `Curator.ApplyBatch` e `Curator.Recover`.
- Interface: `memory_curate_batch_preview` e `memory_curate_batch_apply`; as tools individuais usam o mesmo motor com um item.
- Limite: uma a dezesseis decisoes, proposal IDs distintos e target paths distintos.
- Verificacao normativa: `make verify`.

## Contrato

Uma curation transaction parte de um unico `HEAD`, revalida todos os previews e suas evidencias sob o lock compartilhado e produz um unico commit. Cada proposta selecionada termina como `consumed`, `discarded` ou `pending`; `keep_pending` preserva uma proposta sem alteracao quando outras decisoes do lote podem prosseguir e nao pode formar um lote sozinho. Propostas nao selecionadas permanecem no inbox. Operacoes sobre o mesmo target nao sao compostas implicitamente e devem ocorrer em transacoes separadas.

O commit e construido com indice isolado a partir do parent, enquanto o indice reconciliado parte dos bytes do indice real para preservar staging alheio. O compare-and-swap de `refs/heads/main` continua sendo o unico ponto de publicacao. Nenhum hook ou operacao remota e executado.

## Marcador e recovery

Antes de alterar o worktree, GPM sincroniza:

- no state root privado, um transaction marker `0600` com ID, root, branch, parent, commit, tree, paths, operacoes e hashes de todos os inputs, outputs, previews e evidencia revisada;
- ao lado do indice Git, um artefato de indice reconciliado sem conteudo de blobs, para garantir que o hard link de ownership nunca atravesse filesystems.

Conhecimento, replacement, source excerpt, transcript e local source binding nunca entram no marcador. O `<git-index>.lock` e criado como hard link do artefato registrado; inode e hash provam ownership. Um lock diferente e sempre tratado como estrangeiro e preservado.

Recovery sob o lock compartilhado observa `HEAD`:

- `HEAD == parent`: targets em post-state sao revertidos para o baseline exato, propostas permanecem e artefatos controlados sao removidos;
- `HEAD == commit`: o commit preparado e validado, o indice e instalado ou reconhecido, propostas exatas sao removidas e o mesmo commit e retornado;
- qualquer outro `HEAD`, lock estrangeiro ou terceiro estado falha fechado sem merge ou remocao de estado alheio.

O servidor MCP executa recovery antes de aceitar chamadas quando existe marcador. Apply tambem recupera uma transacao anterior antes de iniciar outra, e um retry do mesmo batch reconhece o commit ja publicado.

Preview, schema, privacidade, targets, referencias e evidencia sao revalidados uma segunda vez depois da aquisicao do index lock e imediatamente antes da primeira escrita no worktree. Mudanca de qualquer hash aborta para o baseline sem publicar.

## Evidencia executavel

Os testes cobrem:

- duas propostas e dois targets em um unico commit de quatro paths;
- preservacao exata de staging e bytes unstaged alheios;
- rejeicao de lote vazio, acima de dezesseis itens, proposal repetida ou target repetido;
- fault injection depois do marcador, do index lock, de cada target, antes/depois de `update-ref`, depois da instalacao do indice e depois de cada remocao de proposta;
- convergencia para baseline antes de `update-ref` e para o mesmo commit depois dele;
- ausencia de conteudo de proposta no marcador;
- preservacao de um Git index lock estrangeiro.
