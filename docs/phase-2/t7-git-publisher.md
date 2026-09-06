# T7: Publisher Git

Fixed point: `aaadcfe` (`feat: add recoverable proposal apply`).

## Seams aprovados

- `publisher.Publish(ctx, ApplyReceipt) -> PublicationReceipt` e a unica mutacao Git nova.
- `proposal publish` consulta o ApplyReceipt, chama o Publisher concreto e entrega o resultado a `proposal.Workflow.RecordPublication`, unico dono da persistencia de receipts.
- `proposal publish <id>` e a fronteira CLI humana; MCP continua sem apply ou publish.
- O adapter Git usa o protocolo `.git/index.lock`, indices temporarios, `update-ref` CAS e marcador privado duravel.

## Tracer bullets

1. Commit valido altera somente o target do ApplyReceipt e preserva staged/unstaged alheio.
2. HEAD, entrada do target e post-state sao revalidados sob `index.lock` antes da captura do blob.
3. Edicao antes da captura aborta; edicao depois da captura permanece unstaged contra o novo HEAD.
4. CAS da branch e instalacao atomica do indice real deixam target limpo e preservam as demais entradas/flags.
5. Lock alheio causa erro sem mutacao; lock proprio so e removido ou concluido com marcador duravel correspondente.
6. Retry depois de falha Git conserva `applied_unpublished`; crash depois do commit reconcilia o mesmo commit sem duplicacao.
7. PublicationReceipt torna publish idempotente e nao contem conteudo de sessao.

Filtros focados: `go test ./internal/publisher -run <tracer> -count=1`, `go test ./internal/proposal -run Publish -count=1` e `go test ./internal/app -run ProposalPublish -count=1`.

## Verificacao final

```text
make verify
make verify-offline
```

## Contrato implementado

Publisher fixa o memory root por descritor, valida o ApplyReceipt e adquire o protocolo real `<git-index>.lock`. O lock nasce por rename sem substituicao de um temporario adjacente que ja contem um token sincronizado. Um marcador `0600` no state root privado registra token, paths controlados, parent, target e hashes; recovery nunca remove um lock cujo conteudo nao corresponda ao token ou ao hash de indice registrado.

Sob o lock, Publisher confirma HEAD, branch simbolica, entrada stage 0 do target, blob/mode da tree e `post_state_hash`. Os bytes sao lidos novamente para a captura; somente esse buffer alimenta `git hash-object`. A tree do commit parte de um indice temporario inicializado no parent e recebe apenas o target. A mensagem e fixa e inclui somente `Git-Push-Memories-Proposal: <id>/<revision>`.

O indice real reconciliado parte dos bytes do indice bloqueado, atualiza somente a entrada do target e e preparado em temporario adjacente ao indice real para que a instalacao seja atomica mesmo quando state root e git dir estao em filesystems distintos. Antes do `update-ref` CAS, o marcador preparado persiste commit, blob e hash do indice final. Depois do CAS, o rename instala o indice e preserva edicoes de worktree posteriores a captura como unstaged.

A CLI consulta o ApplyReceipt no Workflow, chama diretamente o Publisher concreto e devolve o resultado a `Workflow.RecordPublication`, preservando ownership dos receipts sem criar uma interface para a unica implementacao. Publisher esconde toda a finalizacao atras do unico metodo `Publish` e remove o marcador antes de retornar. Se houver interrupcao entre esse cleanup e o receipt, o retry reconhece diretamente o HEAD por parent, trailer, tree, hash do blob e entrada do indice. Se o marcador ainda existir, ele valida tambem branch e indice preparado. O instante e a identidade Git persistidos no intent tornam o objeto commit deterministico em retry anterior ao marcador preparado. Nenhum comando de rede e executado.
