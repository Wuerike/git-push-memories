# E2E de bootstrap da Fase 2

## Resultado

O run classificatorio `20260907T015921Z-bootstrap-e2e` passou em 2026-09-07. Ele avaliou o working tree que introduz o proprio runner T17 sobre o fixed point `8f1ae39`. O manifest registra esse estado como dirty e o vincula pelos hashes dos tres sources executados e do binario, em vez de inferir equivalencia apenas pelo HEAD.

O memory root iniciou como Git worktree com um commit vazio e zero arquivos `*.md`. Nenhum fixture forneceu Markdown ou anchor. O fluxo MCP/CLI executou, nessa ordem:

1. `memory_propose_create_page`, show, apply e publish;
2. `memory_propose_create_section`, show, apply e publish;
3. `memory_propose_replace`, show, apply e publish;
4. `memory_search` em novo processo, code root, HOME, config root e state root.

Cada commit alterou somente `parcel-dispatch.md`. O estado final preservou os IDs gerados `parcel-dispatch-overview` e `parcel-dispatch-failure-modes`, reteve somente o estado atual de tres falhas e foi recuperado em 296 bytes, abaixo do budget de 2048 bytes.

## Privacidade e durabilidade

- seis candidatos invalidos ou sensiveis foram rejeitados;
- o servidor permaneceu utilizavel depois das rejeicoes;
- `strace -ff -e trace=%file` encontrou zero write targets no run de privacidade;
- o state root de privacidade nao foi criado;
- nenhum marcador sintetico apareceu fora do fixture de origem;
- fault filters de create page, create section, pre-rename e publicacao de target novo passaram.

## Verificacao

| Etapa | Exit code | Duracao |
|---|---:|---:|
| bootstrap E2E | 0 | 1408 ms |
| bootstrap fault filters | 0 | 1131 ms |
| `make verify` | 0 | 3220 ms |
| `make verify-offline` | 0 | 1693 ms |
| artifact marker scan | 0 | 0 ms |

A evidencia local ignorada pelo Git esta em `.artifacts/phase-2/20260907T015921Z-bootstrap-e2e`. O checksum do manifest e `918a1805184dcaf5357997a0f2163a05766a1a96e5e3d282482018c64ff5d12c`; o checksum da memoria final e `ca5f0eb7df1f987234c1f51222769f44ffef476d93e8fd12fb9e04560f7546ea`.

T17 fecha o gate tecnico revisado. Ele nao mede valor nem substitui a inferencia autenticada exigida em T19.
