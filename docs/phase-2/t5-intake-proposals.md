# T5: Intake Guard e propostas

## Fixed point e seams

O fixed point de T5 e `19e442d61d4d54b6006581f0db30047951260412`. `internal/intake` recebe o JSON original e e a unica origem de `AllowedCandidate`. `internal/proposal` recebe somente candidatos permitidos e usa `Memory Repository.Preview` para produzir post-state sem escrever Markdown. O Proposal Store permanece privado ao workflow.

Os filtros focados sao `go test ./internal/intake`, `go test ./internal/memory -run TestPreview` e `go test ./internal/proposal`. A matriz final e `make verify` seguida de `make verify-offline`.

## Contratos

- O JSON v1 possui exatamente `target_path`, `section_id`, `replacement` e `source_paths`; todos sao obrigatorios e chaves duplicadas falham.
- O state root e injetado explicitamente, fica fora dos repositorios de memoria e codigo e nao e criado antes de um prepare permitido produzir uma proposta.
- Proposal IDs usam 128 bits aleatorios em hexadecimal; a primeira e unica revisao desta fase e `1`.
- A proposta persiste uma operacao `replace_section_content`, hashes de baseline/post-state, HEAD/blob esperados, diff unificado, source paths sanitizados, timestamp UTC e `pending`. Bytes completos do baseline nao sao persistidos.
- Prepare exige target limpo no index e worktree contra o HEAD capturado. Review apenas recarrega o artefato imutavel.
- Reject aceita somente o enum fechado, publica receipt duravel antes de remover o payload e retorna o mesmo receipt em repeticoes.

O store usa diretorios `0700`, arquivos `0600`, temporario adjacente, `fsync`, rename e `fsync` do diretorio. A inicializacao remove somente temporarios controlados deixados por operacoes interrompidas.

`Preview` repete a verificacao do worktree, index, HEAD, blob e hash antes de retornar. Como T5 nao pode criar `.git/index.lock` sem violar a fronteira read-only, T6 repete essas validacoes antes do apply. A CLI `proposal show` abre o store sem recovery para permanecer estritamente read-only; prepare e reject executam a limpeza controlada de startup.

`memory_propose_replace` nao entra neste incremento: conforme a ordem do plano, T8 adiciona a tool somente depois de T4 e T5 estabilizarem Retrieval, Intake Guard e Proposal Workflow.
