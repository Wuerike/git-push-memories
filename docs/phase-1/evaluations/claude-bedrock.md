# Avaliacao estatica de claude-bedrock

## Identificacao

| Campo | Valor |
|---|---|
| Projeto | claude-bedrock |
| Repositorio | `https://github.com/iurykrieger/claude-bedrock.git` |
| Clone local | `~/workspace/claude-bedrock` |
| Revisao | `7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0` |
| Branch | `main`, alinhada a `origin/main` |
| Estado inicial | arvore limpa |
| Data | 2026-09-04 |
| Avaliacao | triagem estatica, sem execucao do plugin ou dos testes |
| Avaliador | OpenCode |
| Run ID | nao se aplica; nenhuma execucao comportamental |
| Commit inicial da memoria | nao se aplica; repositorios descartaveis nao foram criados |

## Hipotese

claude-bedrock pode informar proposta, revisao e publicacao Git, mas suas garantias estao em prompts interpretados pelo LLM e nao em contratos deterministas.

## Escopo e metodo

Foram inspecionados manifestos, skills, hook Python, scripts, site e testes. Nao foram executados Claude Code, graphify, docling, Git remoto ou testes.

```bash
git rev-parse HEAD
git branch --show-current
git remote get-url origin
git status --short --branch
git log -1 --format='%cI %s'
git rev-list --count HEAD
git rev-list --count --since='2026-08-05T00:00:00Z' HEAD
git shortlog -sne HEAD
```

Os comandos retornaram `0`; a duracao nao foi medida. O produto e principalmente um plugin declarativo em Markdown/YAML, com hook Python e extratores JavaScript; o site usa Next/React (`.claude-plugin/plugin.json:1-23`; `hooks/error_reporter.py:1-525`; `site/package.json:1-31`). O historico local tem 61 commits, nenhum desde 2026-08-05 e autoria concentrada no mantenedor e no bot de release; a revisao e de 2026-05-05, versao 1.6.0.

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). Somente Git e leitura estatica de arquivos foram usados.

## Fluxo estatico

| Estagio | Evidencia no clone | Conclusao |
|---|---|---|
| Entrada | `skills/preserve/SKILL.md:310-474`; `skills/learn/SKILL.md:138-257` | Skills recebem texto, sessao, graphify, URL e arquivos. |
| Filtro | `skills/preserve/SKILL.md:1127-1138`; `hooks/error_reporter.py:252-276` | Proibicao de segredos e prompt; redacao determinista existe apenas no reporter. |
| Persistencia | `skills/preserve/SKILL.md:86-306,582-625` | Graphify e mesclado antes da proposta; entidades Markdown vem depois da confirmacao. |
| Estado atual | `skills/preserve/SKILL.md:507-562,700-736`; `skills/compress/SKILL.md:753-775` | Matching existe, mas varias entidades, links e sources sao append-only. |
| Proposta | `skills/preserve/SKILL.md:582-625`; `skills/sync/SKILL.md:290-316` | Exibe resumo e aceita yes/no/adjust, sem diff, hash ou artefato aplicavel. |
| Git/PR | `skills/preserve/SKILL.md:916-1045` | Suporta commit/push/PR por instrucoes ao agente, com staging amplo e sem drift. |
| Recuperacao | `skills/ask/SKILL.md:155-195,289-297,490-559` | Busca graph-first e fallback Markdown; graph-only cria split-brain. |

## Armazenamento e dependencias

Entidades Markdown e o Git do vault sao o canonico pretendido. `graphify-out/graph.json` e chamado indice, mas pode conter memoria graph-only respondida por `/ask`; nao e reconstruivel apenas das paginas. Outros derivados sao append-only e nem todos entram no commit (`skills/preserve/SKILL.md:242-280,943-956`). Configuracao e registro de vaults persistem fora das entidades (`skills/setup/SKILL.md:574-605,1138-1205`).

O fluxo minimo exige Claude Code e seu LLM, alem de Git para publicacao. Nao exige daemon ou banco. `/learn` exige docling e graphify, instalados sem pin (`skills/learn/SKILL.md:99-134`; `skills/setup/SKILL.md:116-191`). Integracoes Confluence, Google, GitHub e HTTP sao condicionais.

## Privacidade, sessoes e estado atual

`/learn` copia conteudo para `/tmp` e executa ferramentas antes de qualquer scan de segredo (`skills/learn/SKILL.md:158-166,230-387`). O hook Stop recebe transcript e persiste cache/flag com session ID (`hooks/hooks.json:4-10`; `hooks/error_reporter.py:298-355,444-517`). Portanto as garantias de prompt nao satisfazem privacidade antes da primeira persistencia nem ausencia de referencias de sessao.

Actors podem substituir conteudo, mas people/teams/concepts/topics, sources e wikilinks sao append-only. Mudar `team-a` para `team-b` pode deixar backlinks antigos. O graph merge conserva nos e arestas anteriores (`skills/preserve/SKILL.md:162-225,700-736`). O resultado do corpus ainda precisa ser observado.

## Revisao e publicacao

A proposta permite ajuste textual, mas nao contem unified diff, bytes completos, baseline ou ID. Graph derivado ja pode ter mudado antes da resposta. Nao ha deteccao de drift nem rollback transacional.

O fluxo Git adiciona diretorios inteiros, nao apenas arquivos da proposta, e nao mostra o diff staged. `concepts/` e omitido. Na ausencia de `gh`, o modo PR pode regredir para push direto em `main` (`skills/preserve/SKILL.md:943-1000`). O setup nao inicializa Git (`skills/setup/SKILL.md:1288-1296`). Essas sao instrucoes ao LLM, nao invariantes executaveis.

## Claude Code e testes

O plugin expoe oito skills e um hook Stop. Captura e recuperacao sao comandos explicitos `/bedrock:preserve` e `/bedrock:ask`; o hook serve ao reporter, nao a memoria. A integracao precisa de execucao isolada.

Existem testes `unittest` apenas para o reporter, incluindo redacao e issues (`hooks/tests/test_redaction.py:11-89`; `hooks/tests/test_issues.py:16-149`; `hooks/tests/test_main.py:17-74`). Nao ha testes para proposta, consolidacao, Git/PR, drift, rebuild ou skills. O workflow de release nao executa testes (`.github/workflows/release.yml:1-89`).

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa estatica |
|---|---|---|---|
| Markdown e Git canonicos | parcial | [Armazenamento](#armazenamento-e-dependencias) | Graph-only e setup sem Git criam split-brain. |
| Operacao local | falha | [Dependencias](#armazenamento-e-dependencias) | O nucleo depende de Claude Code/LLM remoto. |
| Ausencia de backend obrigatorio | passa | [Dependencias](#armazenamento-e-dependencias) | Nao requer daemon ou banco residente. |
| Indice reconstruivel | falha | [Armazenamento](#armazenamento-e-dependencias) | Nao ha rebuild de `graph.json` apenas do Markdown. |
| Privacidade antes da persistencia | falha | [Privacidade](#privacidade-sessoes-e-estado-atual) | Temp e graph recebem conteudo antes de filtro determinista. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-sessoes-e-estado-atual) | Reporter guarda referencia/cache de sessao. |
| Estado atual | pendente | [Estado atual](#privacidade-sessoes-e-estado-atual) | Regras append-only podem manter ownership antigo; requer corpus. |
| Consolidacao antes da publicacao | parcial | [Revisao](#revisao-e-publicacao) | Graph muda antes da proposta e nao ha artefato aplicavel. |
| Revisao explicita | parcial | [Revisao](#revisao-e-publicacao) | Ha yes/no/adjust, sem diff/baseline e com caminhos autonomos. |
| Repositorio de memoria separado | passa | [Armazenamento](#armazenamento-e-dependencias) | Vaults independem do cwd e cobrem varios repositorios. |
| Integracao com Claude Code | pendente | [Claude Code](#claude-code-e-testes) | Plugin e skills existem; falta ciclo isolado. |
| Testabilidade | falha | [Testes](#claude-code-e-testes) | O nucleo declarativo nao possui contratos automatizados. |
| Licenca compativel | parcial | [Licencas](../licenses.md#claude-bedrock) | Raiz MIT; dependencias sem pin e ativos nao foram auditados. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Riscos e substituicoes estao delimitados; custo alto. |

## Riscos, custo e duvidas

Como base, seria necessario transformar prompts em contratos deterministas, filtrar antes de temp, criar proposta com diff/hash, detectar drift, reconstruir indice, resolver relacoes antigas, isolar staging e testar todo o fluxo. graphify, docling e ativos tambem exigem pin e auditoria.

Fluxos restantes limitados:

1. Etapas 4/5: rejeitar, ajustar, aceitar e repetir a mudanca do corpus, inspecionando backlinks.
2. Passagem de privacidade: `/preserve` e `/learn` desde a primeira exposicao.
3. Etapa 5: graph alterado antes da confirmacao, drift e mudancas nao relacionadas/staged.
4. Etapa 5: commit-only e PR, inclusive ausencia/falha de `gh`.

## Recomendacao preliminar

**Reimplementacao conceitual.** A proposta conversacional, estrategias Git, multiplos vaults e proveniencia sao referencias uteis; nao ha componente determinista pronto para extracao nesta etapa.
