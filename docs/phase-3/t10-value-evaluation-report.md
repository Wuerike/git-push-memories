# T10: acompanhamento da execucao de valor

Status em 2026-09-11: **tentativa v2 encerrada com decisao operacional `revisar_o_fluxo`; avaliacao autenticada nao iniciada**.

A regra 2 de T9 aplica-se a execucao incompleta. Nao houve amostra comparativa nem resultado de valor. A Fase 4 continua sem autorizacao do gate.

## Historico v1: identidade e comando autorizado

| Campo | Valor |
|---|---|
| Protocolo | [T9](t9-value-evaluation-protocol.md), commit `938f38f` |
| Experiment ID | `phase3-curated-memory-value-v1` |
| Produto congelado | `e92a96b6bc4c6f2f169b8b8e96a419c9d9a1b9b2` |
| Runner inicial | `4c279c6` |
| Diagnostico visivel do preflight | `91bd571` |
| Runner da ultima tentativa | `c71674b1ded43a0720f087ed606d73b1ba687f13` |
| Evidence root solicitado | `.artifacts/phase-3/20260911T192349Z-phase3-value` |
| Limite autorizado | USD 25,00 |

```bash
tests/experiment/run-phase-3-value.sh --approved 20260911T192349Z-phase3-value
```

## Historico v1: observacoes operacionais

O runner e seus testes offline foram implementados e commitados antes das tentativas de execucao. O `make verify` passou durante a implementacao.

As primeiras tentativas do launcher falharam no check `tests`, antes do ledger. A saida visivel identificou que o `umask 077` do launcher alterava a criacao de fixtures para `0600`, enquanto dois testes em `internal/memory` esperavam `0644`. O teste focado `TestApplyDoesNotWriteOutsideRootGitIndexOrObjects` reproduziu a falha com `umask 077` e passou com `022`.

O commit `c71674b` aplica `umask 022` somente ao subprocesso dos checks. O processo externo mantem `077` para o TSV e o runtime. `TestLauncherIsolatesVerificationUmaskFromPrivateRuntime` falhou antes da correcao e passou depois, usando executaveis sinteticos sem inferencia.

A ultima tentativa passou os quatro checks offline do launcher:

| Check | Resultado |
|---|---|
| `GOTOOLCHAIN=go1.25.12 GOPROXY=off go test ./...` | passou |
| `GOTOOLCHAIN=go1.25.12 GOPROXY=off go test -race ./...` | passou |
| `GOTOOLCHAIN=go1.25.12 GOPROXY=off go vet ./...` | passou |
| `GOTOOLCHAIN=go1.25.12 GOPROXY=off go build ./...` | passou |

Em seguida, `validateRunEnvironment` encerrou com `runtime version mismatch: git`. A consulta local retornou `git version 2.55.0`; o protocolo exige `2.54.0`. A regra `environment.runtime_version_rule` e a secao de ambiente de T9 exigem novo protocolo para divergencia de versao.

## Historico v1: estado da avaliacao e evidencia

- Nenhum preflight de modelo ou sessao classificatoria autenticada foi iniciado; nenhum custo de inferencia foi incorrido por essas tentativas.
- Capturas, curadorias e pares comparativos nao foram executados.
- O evidence root solicitado e seu `ledger.json` nao foram criados: a falha de versao ocorre antes de `createEvidenceStore`.
- O TSV temporario dos checks foi removido pelo trap do launcher; este documento registra a observacao operacional, nao substitui um relatorio classificatorio com checksums.
- Nao ha score, taxa de utilidade, medida de curadoria, auditoria classificatoria de privacidade ou decisao de valor a reportar.

## Tentativa v2: ambiente compativel e interrupcao no terminal

O [pre-registro v2](t9-value-evaluation-protocol-v2.md), commit `2c94ef1d443bc3c2d644c747eac75c4a69d5f820`, alterou somente o experiment ID para `phase3-curated-memory-value-v2` e a versao de Git para `2.55.0`. O teste de comparacao com v1 e `make verify` passaram antes do commit.

Com autorizacao do usuario para o mesmo limite de USD 25,00, foi executado:

```bash
tests/experiment/run-phase-3-value.sh --approved 20260911-value-v2
```

Os quatro checks offline passaram novamente. O ledger foi criado em `.artifacts/phase-3/20260911-value-v2/ledger.json` com `status=started` em `2026-09-11T20:21:55.65493523Z`.

Ao tentar ler a attestation, o runner recebeu `SIGTTIN`: `timeout 8h` criou um grupo de processos diferente do grupo de primeiro plano do terminal. A inspecao de processos mostrou `go` e `runner` suspensos (`STAT=Tl`), com PGID diferente de TPGID. A declaracao confirmada pelo usuario e enviada ao terminal foi exibida pelo echo do terminal, mas nao foi consumida pelo runner.

A tentativa foi encerrada externamente em `2026-09-11T20:39:49Z`. O ledger original permanece em `started`, com `attempts: []`; ele nao foi reescrito como se o cleanup normal tivesse ocorrido. `report.json` identifica explicitamente a reconciliacao operacional posterior ao encerramento. Nao foi executado `Decide` sobre registros sintetizados; a decisao operacional aplica a regra de execucao incompleta documentada em T9.

| Medida | Observacao v2 |
|---|---|
| Preflight autenticado do modelo iniciado | 0 |
| Sessoes autenticadas iniciadas | 0 |
| Capturas e curadorias executadas | 0 |
| Pares iniciados/concluidos | 0/12 |
| Custo de inferencia | USD 0,00 |
| Avaliacao comparativa | indisponivel |
| Decisao operacional | `revisar_o_fluxo` |

O diretorio temporario de runtime e o auth root tmpfs foram inspecionados e estavam vazios; ambos foram removidos. Nenhuma credencial havia sido copiada. O TSV dos checks nao foi retido porque a interrupcao ocorreu antes de `writePreregistration`. A evidencia preservada contem apenas ledger original, relatorio operacional sanitizado e checksums.

| Artefato v2 | SHA-256 |
|---|---|
| `ledger.json` | `33eec9d68c5d0a35e5150a511abe27e8793996d03697761f49cd80d988e92243` |
| `report.json` | `f9029f0c76f2499fccaf748d104b9cb9d7315161ed597d3cc571433a8183e1b1` |

## Correcao e proximo passo

O launcher foi corrigido para `timeout --foreground 8h`. O teste `TestLauncherIsolatesVerificationUmaskFromPrivateRuntime` agora executa o launcher em um pseudoterminal com runtime sintetico, verifica que o runtime pertence ao grupo de primeiro plano e le uma confirmacao sintetica. Ele falhou com exit code 44 antes da correcao e passou depois.

Essa correcao foi feita depois da interrupcao e nao e apresentada como comportamento observado em v2. A tentativa v2 nao sera substituida. Uma nova rodada exige novo pre-registro antes de inferencia; T10 nao demonstrou valor suficiente para autorizar a Fase 4.
