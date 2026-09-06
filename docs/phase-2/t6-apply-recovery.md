# T6: apply e recuperacao

Fixed point: `8442824` (`feat: add guarded memory proposals`).

## Seams aprovados

- `proposal.Workflow.Apply(ctx, proposalID, revision)` aceita somente a revisao exata.
- `proposal.Workflow.Reject` e `Apply` serializam no mesmo lock multiprocesso por proposta.
- `proposal.Workflow.Review` e `Applied` expoem somente estados duraveis.
- `memory.Repository` classifica baseline, post-state e drift sob o lock exclusivo do memory root.
- `proposal apply` e a fronteira CLI de aceite humano; MCP continua sem autoridade de apply.

## Tracer bullets

1. Apply valido troca apenas a secao e persiste um receipt `applied_unpublished` antes de remover a proposta.
2. Apply repetido retorna o mesmo receipt sem nova escrita.
3. Drift, revisao incorreta, adulteracao e conteudo sensivel falham sem alterar bytes.
4. Falha antes do rename preserva baseline; falha depois do rename recupera pelo post-state; baseline restaurado continua elegivel para retry.
5. Falha depois do receipt termina somente o cleanup.
6. Corridas apply/reject publicam exatamente um estado terminal.
7. Startup remove temporarios controlados e reconcilia apply interrompido sem reparar drift automaticamente.

Filtros focados: `go test ./internal/proposal -run <tracer> -count=1` e, para a fronteira humana, `go test ./internal/app -run ProposalApply -count=1`.

## Verificacao final

```text
make verify
make verify-offline
```

## Contrato implementado

`apply` carrega a proposta e a revisao exatas, verifica o selo SHA-256 do JSON integral, reconstrui o candidato para um novo Intake Guard e recalcula baseline, post-state e diff a partir do blob ligado ao HEAD esperado. O ID e derivado do selo, que inclui um nonce aleatorio; alterar campos mantendo o ID exigiria uma pre-imagem de 128 bits. O rescan, a classificacao e a troca do arquivo ocorrem sob o lock exclusivo multiprocesso do memory root. Baseline e post-state sao os unicos estados reconhecidos; qualquer terceiro hash retorna drift.

Apply e reject adquirem o mesmo arquivo de lock privado `locks/<proposal-id>.lock` antes de observar ou publicar um terminal. O receipt de apply v1 registra proposal ID/revisao, `status: applied_unpublished`, instante, memory root canonico, target, hashes, HEAD e blob de baseline. O payload so e removido depois de temporario, `fsync`, rename sem substituicao e `fsync` do diretorio do receipt.

Na inicializacao mutavel, o reconciliador limpa temporarios controlados, termina cleanup de receipts existentes e examina propostas pendentes sem aplicar baseline. Se os bytes ja forem exatamente o post-state, publica o receipt ausente; baseline continua elegivel para retry e drift permanece sem reparo automatico. A CLI `proposal apply <id> --revision <n>` constitui o aceite humano. Nenhuma tool MCP recebe autoridade de apply.
