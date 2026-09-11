# T10: acompanhamento da execucao de valor

Status em 2026-09-11: **bloqueada no preflight de ambiente; avaliacao autenticada nao iniciada**.

## Identidade e comando autorizado

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

## Observacoes operacionais

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

## Estado da avaliacao e evidencia

- Nenhum preflight de modelo ou sessao classificatoria autenticada foi iniciado; nenhum custo de inferencia foi incorrido por essas tentativas.
- Capturas, curadorias e pares comparativos nao foram executados.
- O evidence root solicitado e seu `ledger.json` nao foram criados: a falha de versao ocorre antes de `createEvidenceStore`.
- O TSV temporario dos checks foi removido pelo trap do launcher; este documento registra a observacao operacional, nao substitui um relatorio classificatorio com checksums.
- Nao ha score, taxa de utilidade, medida de curadoria, auditoria classificatoria de privacidade ou decisao de valor a reportar.

## Proximo passo

T10 permanece em andamento e o inicio da Fase 4 continua sem autorizacao do gate de valor. Prosseguir no ambiente atual exige pre-registrar outro protocolo com a versao de Git disponivel e revisar essa mudanca antes da inferencia. O protocolo v1 e seus criterios continuam preservados no commit de T9.
