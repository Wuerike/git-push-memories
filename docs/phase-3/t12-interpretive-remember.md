# T12: /remember interpretativo

Status: **skill reescrita e verificada pelos testes automatizados; protocolo de aceitacao manual escrito e NAO executado pelo implementador.**

## Hipotese

O `/remember` consegue interpretar o input (sujeito, escopo e forma), transformar um fato sobre uma pessoa em conhecimento do time, avisar quando ja existe proposta pendente do mesmo sujeito e, mesmo assim, criar sempre uma proposta nova. Quem consolida as propostas e o `/curate` agrupado.

## Ponto fixo e seam

- Seam publica: skill `/remember [fato]` com MCP sobre stdio.
- Ferramentas usadas: `memory_search`, `memory_expand`, `memory_proposal_list`, `memory_reference_snapshot`, `memory_reference_expand` e `memory_propose`. Nenhuma ferramenta nova.
- Origem: experimento de 2026-10-20 em que o primeiro `/remember` gerou titulo e conhecimento sobre a pessoa, nao sobre o time.
- Fora de escopo: atualizar ou substituir proposta ja criada, e o fluxo de release multi-time. Ficam no Plano C (`.local/design-2026-10-22-c-release-flow-notes.md`).

## Limite conhecido

`memory_proposal_list` le as propostas do checkout do repositorio de memoria. O aviso de proposta pendente enxerga somente o que esta commitado e legivel ali.

## Cenario

Repositorio de memoria descartavel, sem nenhuma pagina previa, com os dois prompts originais usados textualmente:

```text
/remember meu nome é wuerike e eu so do time da camada de leitura
/remember o time de camada de leitura tem outro membro que se chama guilherme
```

## Rubrica

- R1 o primeiro `/remember` mostra a interpretacao e propoe titulo do **time** (nao da pessoa), com o membro numa lista;
- R2 o segundo `/remember` identifica a proposta pendente do time, avisa com titulo e ID e, apos confirmar, cria uma **segunda** proposta (o inbox tem duas);
- R3 `/curate` agrupa as duas e produz **uma** pagina final correta com os dois membros (usa o Plano A, ADR 0007).

## Cenarios extras

- S1 `/remember "restructure ..."` sobre pagina existente: a skill nao trata como fato pessoal e gera proposta `update`;
- S2 input com dois sujeitos diferentes: a skill pede um `/remember` por sujeito;
- S3 input pessoal ("prefiro tabs"): a skill avisa que nao pertence a memoria compartilhada e pede decisao;
- S4 existe proposta pendente do mesmo sujeito e a pessoa escolhe criar mesmo assim: a skill cria nova sem tentar alterar a antiga.

## Verificacao

Em cada item registrar passou ou falhou e guardar o transcript em `.local/`. Garantias comuns a todos os cenarios:

- nenhuma chamada de escrita antes da confirmacao dos campos exatos;
- no maximo uma chamada a `memory_propose` por execucao;
- `memory_search` e `memory_proposal_list` chamadas uma vez cada, somente para classificar.
