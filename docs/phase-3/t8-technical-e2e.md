# T8: E2E tecnico da Fase 3B

Status: implementada e verificada em 2026-09-11.

## Ponto fixo e comando

- Base fixa: `6ffcd26` (`feat: recover multi-path curation transactions`).
- Tracer: `tests/e2e/phase3_test.go`, usando o binario `gpm`, setup real e processos MCP stdio separados.
- Execucao focada: `GOTOOLCHAIN=go1.25.12 GOPROXY=off go test ./tests/e2e -run '^TestPhase3TechnicalWorkflow$' -count=1 -v`.
- Run reproduzivel com evidencia sanitizada: `tests/e2e/run-phase-3.sh [run-id]`.
- Matriz normativa: `make verify`; a run classificatoria tambem executa `make verify-offline`.

## Cenario vertical

O teste parte de repositorios Git descartaveis sem remotos e de um `gpm.json` com categorias configuradas. O binario real executa `gpm setup`; um `claude` sintetico aceita apenas o registro MCP, sem sessao ou transcript.

Processos MCP independentes exercitam:

1. rejeicao de um segredo sintetico antes de criar state root, proposta ou commit;
2. snapshot e expansao transitoria de uma fonte commitada;
3. proposta com referencias `verified` e `mismatch`, proposta com source path `legacy_unverified` e dedupe byte-identico sem novo commit;
4. criacao de duas paginas e consumo de duas propostas em um unico commit;
5. `create_section`, seguido de `remove_section` interrompido antes de `update-ref`, recovery para o baseline e retry confirmado;
6. observacao simultanea do fato canonico antigo e de uma proposta contraditoria;
7. preview em lote de `replace_section`, `remove_page` e `discard_proposal`, incluindo referencia `unavailable` depois do restart;
8. `SIGKILL` deterministico do processo MCP imediatamente depois do `update-ref` concluido;
9. inspecao do commit completo, marcador `0600` sem conteudo, index lock e artefato de indice com o mesmo inode;
10. recovery automatico no startup de outro processo, sem commit filho;
11. recuperacao read-only do estado final em um usuario novo, sem state root anterior ou source binding.

Os crashes sao produzidos por um shim de `git` pertencente ao teste, armado somente para o `update-ref refs/heads/main`: uma vez antes de executar o comando e uma vez depois de sua conclusao. O mesmo shim rejeita e registra tentativas de fetch, pull, push, clone, ls-remote, merge ou URL remota. Nenhum fault hook e exposto no produto.

## Evidencia observada

- `create_page`, `create_section`, `replace_section`, `remove_section`, `remove_page` e `discard_proposal` produzem o estado esperado;
- o duplicate retorna o mesmo ID e nao avanca `HEAD`;
- o canônico antigo e a proposta nova ficam visiveis juntos para a decisao de contradicao, sem classificacao autonoma pelo servidor;
- integridade de referencias cobre `verified`, `mismatch`, `legacy_unverified` e `unavailable`, sempre com semantic support `not_assessed`;
- o commit interrompido possui exatamente os dois targets e as tres propostas decididas;
- restart pre-publicacao restaura baseline e proposta para retry; restart pos-publicacao instala o indice preparado, remove propostas exatas e preserva staging, flags, stat do indice e bytes unstaged alheios;
- marker, index lock e artefato controlado desaparecem depois do recovery;
- commit hooks nao executam e nenhum repositorio sintetico possui remote;
- segredo rejeitado, trecho privado da fonte, root local e preview token nao aparecem no marcador, repositorio de memoria ou state root;
- a recuperacao final retorna somente o timeout novo e nao retorna a pagina removida nem propostas.

Durante a primeira passagem Red, o E2E encontrou que `remove_page` registrava `post_state_git_mode` mesmo sem post-state, tornando o marcador invalido depois de um crash real. T8 corrigiu a construcao do marcador e preserva esse fluxo como regressao.

## Artefatos

O runner grava somente manifest sanitizado, logs dos comandos, matriz de exit codes/duracao e checksums em `.artifacts/phase-3/<run-id>/`, que permanece ignorado pelo Git. Nao sao preservados payloads MCP, preview tokens, source excerpts, marcador bruto, indice binario, propostas, prompts, respostas ou transcripts.
