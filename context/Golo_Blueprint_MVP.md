# Golo - Blueprint do Produto e MVP

> **Nome oficial do projeto: Golo.** Este documento é a fonte de verdade técnica e de produto para a primeira versão.

| Campo | Valor |
|---|---|
| Documento | Product + Technical Blueprint |
| Versão | 2.0 |
| Status | Reposicionado após medição — ver §0 |
| Data de referência | 26 de julho de 2026 |
| Responsável | Enzo Trichês |
| Objetivo central | Medir, com rigor estatístico, se um cenário de jogo se repete com frequência suficiente para valer um preço — e dizer quando não vale |
| Mercado inicial | Ferramenta de validação de cenários para quem analisa futebol ao vivo |
| Princípio | A régua vale mais que o palpite; nunca promessa de lucro |

---

## 0. Por que este documento mudou

A versão 1.0 assumia que estimar bem a probabilidade de gol produziria valor. **Isso foi medido e não se sustentou.** O produto foi reposicionado em torno do que a medição mostrou ser genuinamente forte.

### O que foi medido

Sobre 3.287 partidas reais (Brasileirão, Libertadores, MLS, Liga MX), com 80.261 chutes datados:

| Horizonte | Brier do modelo | Brier da taxa constante | Vantagem (IC 95%, agrupado por partida) |
|---|---|---|---|
| 5 min | 0,11574 | 0,11574 | indistinguível de ruído |
| 10 min | 0,18365 | 0,18361 | indistinguível de ruído |
| até o fim | 0,15036 | 0,15042 | `-0,000066` → `[-0,001025, +0,000853]` |

**O modelo empata com "chutar a taxa média do futebol".** Alimentá-lo com dados de chute reais melhorou o Brier de 0,15130 para 0,15036 — e essa melhora não sobrevive a um teste de significância.

### As três conclusões que reposicionam o produto

1. **Não há vantagem contra o mercado.** O sinal está quase todo na taxa base, que qualquer casa de apostas também conhece. Vender "no que apostar" seria vender ruído com decimais.
2. **Dado melhor não resolve.** Comprar xG melhoraria a precisão absoluta, mas as casas têm o mesmo dado ou melhor. Vantagem vem de ter algo que o outro lado não tem, não de ter algo bom.
3. **O que ficou excelente foi o aparato de medição.** Intervalos de confiança, agrupamento por partida, recusa em falar sem evidência, testes selados. Isso é raro — e é a única peça que não compete com Opta nem com bet365.

### O reposicionamento

De: *"prevemos gols melhor que os outros"* — afirmação que a medição não sustenta.

Para: **"você tem uma teoria sobre futebol; nós dizemos se ela é verdade, em segundos, sobre milhares de partidas, sem você apostar um centavo."**

O método correto que circula entre apostadores — definir um cenário, observar 50 a 100 vezes anotando tudo, calcular a taxa de acerto, derivar a odd mínima como `1/taxa` — está certo em estrutura e é inviável na prática. Leva meses e dinheiro real. E 50 observações não bastam: para provar uma vantagem de 5 pontos percentuais sobre uma odd 1,50 são necessárias **~940** observações.

O Golo faz esse trabalho em segundos, com 30 vezes mais amostra e risco zero. Essa é a proposta de valor, e ela **não depende de o modelo vencer ninguém**.

---

## 1. Resumo executivo

O Golo é uma ferramenta que responde a uma pergunta: **"essa minha teoria sobre futebol é verdade?"** — e responde sem que o usuário precise apostar para descobrir.

O usuário monta uma regra na tela: *quando* o cenário ocorre e *o que* se espera dele.

> Quando: passou dos 70 minutos **e** um time está ganhando por 2 ou mais
> Espera-se: sai mais um gol até o fim

O Golo varre milhares de partidas históricas em segundos e devolve:

```
70min, vencendo por 2+
  ocorreu em          1.356 partidas
  saiu mais um gol    50,4%
  intervalo 95%       47,8% - 53,1%
  odd mínima          2,09   (no pior caso do intervalo)
  pior sequência      8 perdas seguidas
  banca sugerida      2,5% por entrada
```

A leitura é imediata: **esse cenário só dá lucro em odd acima de 2,09.** A uma odd de 1,50 ele é prejuízo no longo prazo — não por azar, por aritmética.

Em segundo plano, e alimentando a mesma régua, o Golo mantém um estado probabilístico versionado por partida ao vivo, com probabilidade de gol em múltiplos horizontes. Essa camada continua existindo: ela é o que permite reconhecer um cenário no momento em que ele acontece. Ela deixou de ser o produto.

O MVP não é casa de apostas, não recebe dinheiro, não executa apostas, não recomenda entradas e não promete ganho financeiro.

### As três coisas que ninguém mais entrega

1. **O intervalo de confiança.** Qualquer um mostra "acerto de 50%". Quase ninguém mostra que, com 50 observações, isso significa "entre 36% e 63%" — e que portanto a odd mínima real está entre 1,57 e 2,74. É a diferença entre achar que tem vantagem e ter.
2. **A pior sequência real.** Observada, não estimada. É dela que sai o tamanho da entrada, e é o que quebra a banca de quem não olhou.
3. **O veredito de "não vale".** Quando nenhum cenário supera o preço disponível, o Golo diz isso com todas as letras. Uma ferramenta que só sabe encontrar oportunidade não é ferramenta, é vitrine.

A arquitetura de menor custo usa:

- **Firebase Hosting** para o frontend estático;
- **Firebase Realtime Database** para publicar o estado e as previsões ao vivo;
- **uma VM Compute Engine `e2-micro`** nos Estados Unidos para coleta, normalização, processamento e inferência contínua;
- **SQLite** como banco operacional local;
- **Cloud Storage** para arquivos crus e datasets compactados;
- **BigQuery** somente para análises históricas e avaliação;
- **Python/Colab** para treinamento dos modelos;
- **Go** para o processo de produção de baixa memória e alta previsibilidade.

O custo GCP esperado do MVP, dentro dos níveis gratuitos, é dominado pelo IPv4 externo da VM: aproximadamente **US$ 3,65/mês** para 730 horas. O benefício do Google AI Pro inclui **US$ 10 mensais em créditos de GenAI e Google Cloud**, desde que o benefício esteja resgatado e vinculado à conta de faturamento correta. Os créditos de IA usados em produtos como Flow/Antigravity não devem ser tratados como saldo de infraestrutura do GCP.

### Decisões irrevogáveis do MVP

| Tema | Decisão |
|---|---|
| Arquitetura | Monólito modular, não microserviços |
| Runtime principal | Go |
| Treinamento | Python em máquina local ou Google Colab |
| Banco operacional | SQLite em modo WAL |
| Realtime | Firebase Realtime Database |
| Frontend | React + TypeScript + Vite, publicado no Firebase Hosting |
| Compute contínuo | Compute Engine `e2-micro`, região `us-central1` |
| Modelo inicial | Hazard/Poisson dinâmico + regressão logística calibrada |
| LLM na previsão | Não usar |
| Aposta automática | Fora do MVP |
| Produto vendido | A régua de medição, não a previsão (ADR-008) |
| Autoria do cenário | Do usuário; o Golo mede e reconhece, não propõe (§13) |
| Qualificação | Exige intervalo de confiança, nunca comparação pontual (ADR-009) |
| Taxa de acerto sem intervalo | Proibida em qualquer superfície |
| Kafka, Kubernetes, Cloud SQL | Fora do MVP |
| Cobertura inicial | Uma ou poucas ligas com feed de qualidade conhecida |
| Fonte ao vivo | Adapter substituível; API autorizada é preferível a scraping |

---

## 2. Visão do produto

### 2.1 Problema

Quem analisa futebol acumula teorias: *"jogo empatado no fim tende a abrir"*, *"time vencendo por dois se recolhe"*, *"depois de uma expulsão sai gol"*. Algumas são verdadeiras, outras são folclore, e **nada na experiência normal de assistir futebol distingue as duas**.

O método correto para separá-las é conhecido e circula entre apostadores: defina um cenário específico, observe-o dezenas de vezes anotando tudo, calcule a taxa de acerto, derive a odd mínima como `1/taxa`. O método está estruturalmente certo. Ele é inviável por dois motivos:

1. **É lento e caro.** Cinquenta observações exigem meses e dinheiro real em jogo.
2. **Cinquenta não bastam, e ninguém diz isso.** Com 50 observações a 50% de acerto, a taxa real está entre 36,5% e 63,5%. A "odd mínima de 2,00" é na verdade um intervalo de 1,57 a 2,74. Apostar em 2,10 acreditando ter vantagem pode ser prejuízo garantido. Para provar uma vantagem de 5 pontos percentuais sobre uma odd 1,50 são necessárias ~940 observações.

O erro que o mercado comete não é calcular errado a taxa de acerto. É **apresentá-la sem a incerteza** — e decidir dinheiro com base num número que não suporta a decisão.

### 2.2 Proposta de valor

> Você tem uma teoria sobre futebol. O Golo diz se ela é verdade — em segundos, sobre milhares de partidas, com a incerteza na frente, e sem você arriscar nada para descobrir.

E, criticamente: **diz também quando não é verdade.** O veredito negativo é um resultado do produto, não uma falha dele.

### 2.3 Usuário primário

Quem já analisa futebol ao vivo e quer parar de decidir por intuição:

- tem teorias sobre padrões de jogo e quer testá-las;
- entende que taxa de acerto importa, mas não tem como medi-la sem apostar;
- precisa saber a que preço um cenário deixa de compensar;
- aceita — e prefere — ouvir que a teoria dele não se sustenta.

Esse último ponto é uma escolha de posicionamento com consequência comercial, e está registrada aqui deliberadamente: **o mercado de dicas recompensa confiança, não calibração.** Quem compra palpite quer certeza. O Golo vende o oposto disso, e portanto não disputa esse público.

### 2.4 Usuários futuros possíveis

- criadores de conteúdo estatístico que precisam de números defensáveis;
- portais esportivos que queiram um widget de leitura de jogo ao vivo;
- analistas quantitativos que queiram validar hipóteses rapidamente.

### 2.5 O que o Golo deliberadamente não disputa

| Território | Por que não |
|---|---|
| Probabilidade ao vivo como produto | Commodity. Opta e Stats Perform vendem, com dado melhor e décadas de vantagem. |
| Dicas de aposta | Sem vantagem medida (§0). Vender seria vender ruído. |
| Venda de dados | O dataset é derivado da SportMonks; os termos proíbem redistribuição. |
| Ser mais preciso que o mercado | Medido e refutado. A eficiência do mercado de gol ao vivo é alta. |

---

## 3. Objetivos, não objetivos e critérios de sucesso

### 3.1 Objetivos do MVP

1. Permitir que o usuário defina um cenário de jogo sem escrever código.
2. Medir a frequência histórica desse cenário sobre milhares de partidas, em segundos.
3. **Devolver sempre o intervalo de confiança, nunca só o ponto.**
4. Derivar a odd mínima necessária a partir do limite pessimista do intervalo.
5. Reportar a pior sequência de perdas observada, e dimensionar a entrada a partir dela.
6. Dizer com clareza quando a amostra é insuficiente ou quando o cenário não compensa.
7. Reconhecer ao vivo quando um cenário monitorado pelo usuário está ocorrendo.
8. Manter tudo auditável: entradas, features, versão do modelo, saída e partição de dados.

### 3.1.1 Regras invioláveis da medição

Estas não são preferências de implementação. São o produto.

| Regra | Motivo |
|---|---|
| Uma observação por partida | Um cenário que vale por vinte minutos é **uma** oportunidade de entrada, não vinte. Contar por minuto inflou uma amostra de 255 partidas para 6.284 "observações" e produziu um intervalo de 2 pontos — precisão falsa. |
| Intervalo agrupado por partida | Previsões dentro de uma mesma partida dividem um único desfecho. O n efetivo é o número de partidas. |
| Veredito pelo limite inferior | Usar a estimativa pontual é como um cara-ou-coroa vira "vantagem". |
| Sequência de perdas observada, não estimada | Dimensionar banca por um pior caso imaginado é adivinhação com aparência de método. |
| Amostra insuficiente é dito, não escondido | Um número sem contexto de amostra convida a decisão errada. |

### 3.2 Não objetivos do MVP

- automatizar apostas;
- integrar conta ou saldo de casa de apostas;
- recomendar valores de stake;
- vender assinatura;
- cobrir todas as ligas;
- prever placar exato;
- usar redes neurais profundas;
- usar LLM para calcular probabilidade;
- criar marketplace ou rede social;
- garantir retorno financeiro;
- contornar bloqueios, autenticação ou termos de uso de providers.

### 3.3 North Star

**A medição está correta e é reproduzível** — não "o modelo é bom".

A mudança é deliberada. A North Star anterior (Brier da probabilidade de gol) media a qualidade da previsão, que §0 mostrou empatar com a taxa base. A régua não precisa que o modelo vença ninguém para valer; precisa estar certa.

Concretamente, a North Star é verificada por:

- o número de ocorrências de um cenário bate com uma contagem independente em SQL sobre o mesmo dataset;
- o intervalo estreita conforme a amostra cresce, na proporção esperada;
- rodar duas vezes sobre o mesmo dataset produz o mesmo resultado, com o mesmo hash de partição.

Brier e calibração continuam sendo medidos, e continuam importando — mas como **controle de qualidade da camada ao vivo**, não como promessa ao usuário.

### 3.4 Critérios de sucesso do MVP

O MVP é validado quando:

- o usuário monta um cenário pela interface, sem código, e recebe resposta em segundos;
- toda taxa de acerto exibida vem acompanhada de intervalo e contagem de partidas;
- a odd mínima é derivada do limite pessimista, e isso está explícito na tela;
- um cenário sem amostra suficiente é rotulado como tal, e não recebe veredito;
- um cenário que não compensa a nenhum preço razoável é reportado como tal;
- o replay e o modo ao vivo produzem features equivalentes para o mesmo instante;
- falhas de provider não geram números silenciosamente incorretos.

### 3.5 Gate para monetizar

A cobrança só deve ocorrer quando houver:

1. interface de cenário utilizável por quem não programa;
2. medição verificada contra contagem independente;
3. revisão jurídica específica (§23) — vender ferramenta a apostadores ainda alcança as regras de publicidade de julho/2026;
4. custo de provider compatível com o preço praticável.

**Não há gate de "o modelo precisa ter vantagem".** Esse gate foi removido conscientemente: o produto passou a ser a régua, e uma régua honesta que mede "não há vantagem aqui" está entregando exatamente o que promete.

---

## 4. Escopo funcional do MVP

### 4.0 Tela principal - Laboratório de Cenários

**É a tela do produto.** Tudo o mais existe para alimentá-la.

**Montagem.** O usuário compõe um gatilho a partir de condições legíveis, sem escrever código:

| Dimensão | Exemplos |
|---|---|
| Tempo | a partir do minuto N; nos últimos N minutos |
| Placar | empatado; diferença de exatamente N; alguém vencendo por N+ |
| Total de gols | exatamente N gols na partida até agora |
| Cartões | há expulsão em campo |
| Competição | restringir a uma ou mais ligas |

E escolhe o que se espera: **sai mais um gol até o fim**, ou **dentro dos próximos N minutos**.

**Resultado.** Executado sobre o dataset histórico, sempre com estes campos — nenhum é opcional:

| Campo | Por quê |
|---|---|
| Ocorrências e partidas | O segundo é o tamanho efetivo da amostra |
| Taxa de acerto | O que o usuário veio buscar |
| **Intervalo de 95%** | O que separa vantagem de sorte |
| **Odd mínima (pessimista)** | `1 / limite inferior` — o preço que realmente compensa |
| Pior sequência observada | Base para o tamanho da entrada |
| Fração de banca sugerida | `drawdown aceitável ÷ pior sequência` |
| Veredito em linguagem clara | Incluindo "não compensa" e "amostra insuficiente" |

**Comparação obrigatória com a taxa base.** Todo cenário é exibido ao lado da frequência do mesmo desfecho sem nenhum gatilho. Um cenário que não supera a taxa base não está informando nada — e o usuário precisa ver as duas colunas lado a lado para perceber isso.

### 4.0.1 Monitoramento ao vivo de cenário salvo

O usuário salva um cenário validado e o Golo avisa quando ele ocorre numa partida ao vivo, por Telegram ou na interface.

A mensagem informa: qual cenário disparou, a partida e o minuto, a taxa histórica com intervalo, e a odd mínima necessária.

**A mensagem nunca diz para apostar.** A formulação é *"o cenário que você monitora está acontecendo; historicamente ele dá X%, o que exige odd acima de Y"*. A decisão, o preço e o risco são do usuário. Ver §13 e §23.

### 4.1 Tela de apoio - Live Board

Mantida. Deixou de ser o produto e passou a ser a camada que reconhece cenários ao vivo, além de leitura de jogo por si só.

A tela lista partidas monitoradas com:

- competição;
- mandante e visitante;
- placar;
- minuto e período;
- probabilidade de gol até o fim;
- probabilidade nos próximos 5 minutos;
- qualidade do feed;
- atraso estimado;
- momento da última atualização;
- status: `LIVE`, `PAUSED`, `STALE`, `FINISHED` ou `ERROR`.

### 4.2 Tela da partida

A tela detalhada mostra:

- gráfico da probabilidade ao longo do jogo;
- probabilidades por horizonte;
- mandante versus visitante;
- eventos recentes;
- chutes, chutes no alvo, xG e cartões, quando disponíveis;
- principais features que alteraram a previsão;
- nível de confiança de dados;
- versão do modelo;
- aviso quando o feed estiver atrasado;
- comparação opcional com probabilidade implícita do mercado.

### 4.3 Tela de avaliação

Uma tela administrativa exibe:

- Brier Score;
- Log Loss;
- Expected Calibration Error;
- curva de calibração;
- métricas por liga;
- métricas por faixa de minuto;
- métricas por estado do placar;
- métricas por versão de modelo;
- quantidade de snapshots;
- taxa de feed atrasado ou incompleto.

### 4.4 Modo replay

O usuário seleciona uma partida histórica e reproduz os eventos em velocidade configurável:

- `1x` para reprodução realista;
- `10x` para inspeção;
- `max` para backtest.

O replay usa o mesmo reducer, feature engine e predictor do modo ao vivo.

### 4.5 Features P0

- [ ] seleção manual de partidas monitoradas;
- [ ] coleta de fixtures e eventos;
- [ ] normalização para esquema canônico;
- [ ] reducer determinístico;
- [ ] rolling windows;
- [ ] previsão para 5 minutos, 10 minutos e até o fim;
- [ ] persistência das previsões;
- [ ] publicação no Realtime Database;
- [ ] dashboard ao vivo;
- [ ] replay de partida;
- [ ] relatório básico de calibração;
- [ ] health check do provider;
- [ ] budget alerts no GCP.

### 4.6 Features P1, após validação

- probabilidades separadas por equipe;
- comparação com odds sem margem;
- ensemble de modelos;
- explicação local por feature;
- alertas configuráveis;
- múltiplos providers;
- modo sombra para candidato a novo modelo;
- login Firebase;
- painel de qualidade de dados.

### 4.7 Fora do escopo até decisão jurídica e de produto

- links de afiliado;
- publicidade de casas de apostas;
- execução de apostas;
- copy trading;
- cobrança baseada em lucro;
- notificações que incentivem urgência ou perda de controle;
- acesso por menores de 18 anos.

---

## 5. Definição matemática do problema

### 5.1 Variável alvo

No instante `t` de uma partida, o sistema observa somente informações disponíveis até `t` e estima:

```text
P(existe pelo menos um gol entre t e T | estado e histórico observados até t)
```

Para um horizonte `h`:

```text
y_h(t) = 1, se ocorreu um gol no intervalo (t, t+h]
         0, caso contrário
```

Horizontes do MVP:

- `h = 5 minutos`;
- `h = 10 minutos`;
- `T = fim da partida`.

A interface pode mostrar um rótulo binário, mas a fonte de verdade é sempre a probabilidade contínua.

### 5.2 Processo de intensidade

Se `lambda(u | X_u)` representa a intensidade instantânea de gol, então:

```text
P(nenhum gol entre t e T) = exp(- integral_t^T lambda(u | X_u) du)

P(pelo menos um gol entre t e T) = 1 - exp(- integral_t^T lambda(u | X_u) du)
```

Na implementação discreta, com intervalos pequenos:

```text
P(gol no horizonte) = 1 - exp(- soma_k lambda_k * delta_t)
```

### 5.3 Prior pré-jogo

O modelo começa com uma intensidade pré-jogo baseada em:

- força ofensiva do mandante;
- força defensiva do visitante;
- força ofensiva do visitante;
- força defensiva do mandante;
- vantagem de jogar em casa;
- média da competição;
- escalação ou ausência, quando houver dado confiável;
- expectativa total de gols do mercado, somente no modelo que usa mercado.

Uma formulação inicial:

```text
log(lambda_home_pre) = league_intercept
                       + attack_home_team
                       + defense_away_team
                       + home_advantage

log(lambda_away_pre) = league_intercept
                       + attack_away_team
                       + defense_home_team
```

### 5.4 Atualização ao vivo

A intensidade pré-jogo é alterada por um multiplicador ao vivo:

```text
lambda_t = lambda_pre * exp(beta dot X_t)
```

`X_t` inclui minuto, placar, cartões, pressão recente, chutes e outras features disponíveis.

### 5.5 Modelo inicial recomendado

O MVP deve possuir dois modelos:

#### Modelo A - Baseline estrutural

- Poisson/hazard por equipe;
- minuto e tempo restante;
- placar;
- força das equipes;
- cartão vermelho;
- simples, rápido e interpretável.

#### Modelo B - Regressão logística discreta calibrada

Cada snapshot é uma linha. Para cada horizonte:

```text
logit(P(y_h = 1)) = beta_0 + beta_1*x_1 + ... + beta_n*x_n
```

A versão seguinte pode substituir ou complementar a regressão por LightGBM. O baseline nunca deve ser removido, pois funciona como controle e fallback.

### 5.6 Por que não começar com deep learning

- a qualidade do feed é mais importante que a complexidade do modelo;
- redes sequenciais aumentam o risco de leakage e overfitting;
- interpretação e calibração são mais difíceis;
- a inferência e o treinamento são mais caros;
- um modelo simples revela rapidamente se o sinal existe.

### 5.7 Calibração

As probabilidades devem ser calibradas com dados fora do treino:

- Platt scaling como primeira opção;
- isotonic regression quando houver volume suficiente;
- calibração separada por horizonte;
- calibração por liga somente quando existir amostra suficiente.

Exemplo de interpretação:

> Entre previsões próximas de 60%, aproximadamente 60% devem terminar com gol no horizonte definido.

### 5.8 Incerteza e confiança

A aplicação deve separar:

- **probabilidade do evento**: chance estimada de gol;
- **confiança operacional**: confiança nos dados usados para calcular a probabilidade.

Uma previsão de 70% com feed atrasado não deve ser exibida como equivalente a uma previsão de 70% com feed completo.

---

## 6. Features do MVP

### 6.1 Features de contexto

- competição e temporada;
- mandante/visitante;
- minuto e segundo;
- período;
- tempo restante estimado;
- placar;
- diferença de gols;
- resultado atual: vencendo, empatando ou perdendo;
- expulsões por equipe;
- diferença de jogadores;
- força Elo ou rating próprio;
- intensidade pré-jogo.

### 6.2 Janelas móveis

Calcular janelas de:

- 60 segundos;
- 3 minutos;
- 5 minutos;
- 10 minutos.

Para cada janela, quando o provider permitir:

- chutes;
- chutes no alvo;
- chutes bloqueados;
- xG acumulado;
- ataques perigosos;
- entradas no terço final;
- escanteios;
- posse;
- faltas próximas da área;
- toques na área;
- diferença entre mandante e visitante.

### 6.3 Features temporais

- tempo desde o último chute;
- tempo desde o último chute no alvo;
- tempo desde o último gol;
- tempo desde o último cartão;
- tempo desde a última substituição;
- velocidade de crescimento do xG;
- mudança de intensidade entre as janelas de 10 e 3 minutos.

### 6.4 Features de mercado opcionais

Criar dois conjuntos de modelos:

1. **sports-only**, sem odds;
2. **sports-plus-market**, com odds timestampadas.

Features possíveis:

- probabilidade implícita sem margem;
- variação da odd em 30 segundos, 1 minuto e 5 minutos;
- dispersão entre casas;
- mercado suspenso;
- diferença entre probabilidade do modelo e do mercado.

As odds nunca podem ser associadas a um evento recebido depois do instante da previsão. O timestamp do feed de odds precisa ser preservado.

### 6.5 Dados ausentes

Cada feature deve possuir:

- valor;
- indicador de ausência;
- timestamp de atualização;
- origem.

Nunca preencher silenciosamente dado ausente com zero quando zero possui significado real.

---

## 7. Dados e providers

### 7.1 Princípio

O sistema é provider-agnostic. Nenhum tipo interno deve depender do JSON original de uma API.

### 7.2 Estratégia de dados em três trilhas

#### Trilha 1 - Desenvolvimento histórico

Usar dados abertos para criar o pipeline e testar features. O StatsBomb Open Data disponibiliza eventos e escalações de competições selecionadas para pesquisa, mas não deve ser confundido com um feed ao vivo completo.

#### Trilha 2 - Piloto ao vivo

Usar trial ou plano pequeno de uma API esportiva que possua:

- fixtures;
- relógio da partida;
- placar;
- gols e cartões;
- eventos com timestamps;
- estatísticas ao vivo;
- documentação de limites;
- termos que permitam o uso pretendido.

Um candidato atual é o SportMonks, que oferece trial de 14 dias e planos a partir de aproximadamente EUR 29/mês para cinco ligas. Isso é apenas uma referência de mercado; o adapter deve permitir troca imediata.

#### Trilha 3 - Scraping experimental

Scraping pode ser usado somente como experimento privado quando permitido pelos termos e pela legislação. Não deve ser a fonte única de produção porque:

- o HTML muda;
- pode existir atraso deliberado;
- bloqueios e CAPTCHAs quebram a disponibilidade;
- termos de uso podem proibir redistribuição;
- timestamps são menos confiáveis;
- a origem pode corrigir eventos sem aviso.

### 7.3 Interface do adapter

```go
type Provider interface {
    Name() string
    ListLiveMatches(ctx context.Context) ([]ProviderMatch, error)
    FetchMatchSnapshot(ctx context.Context, providerMatchID string) (ProviderSnapshot, error)
    FetchEventsSince(ctx context.Context, providerMatchID string, cursor string) ([]ProviderEvent, string, error)
    Health(ctx context.Context) ProviderHealth
}
```

### 7.4 Evento canônico

```go
type MatchEvent struct {
    EventID          string          `json:"eventId"`
    MatchID          string          `json:"matchId"`
    Provider         string          `json:"provider"`
    ProviderEventID  string          `json:"providerEventId"`
    EventType        string          `json:"eventType"`
    TeamID           *string         `json:"teamId,omitempty"`
    PlayerID         *string         `json:"playerId,omitempty"`
    MatchSecond      int             `json:"matchSecond"`
    Period           int             `json:"period"`
    ProviderTime     time.Time       `json:"providerTime"`
    ReceivedAt       time.Time       `json:"receivedAt"`
    ProcessedAt      time.Time       `json:"processedAt"`
    XG               *float64        `json:"xg,omitempty"`
    PositionX        *float64        `json:"positionX,omitempty"`
    PositionY        *float64        `json:"positionY,omitempty"`
    Attributes       json.RawMessage `json:"attributes"`
    SchemaVersion    int             `json:"schemaVersion"`
}
```

### 7.5 Tempos obrigatórios

Todo evento deve distinguir:

1. `match_second`: posição no relógio da partida;
2. `provider_time`: horário informado pela origem;
3. `received_at`: chegada ao sistema;
4. `processed_at`: conclusão do processamento.

### 7.6 Deduplicação

Prioridade da chave:

```text
provider + provider_match_id + provider_event_id
```

Fallback por fingerprint:

```text
SHA256(match_id + event_type + team_id + player_id + match_second + coordinates)
```

### 7.7 Correções e eventos reversíveis

O domínio deve suportar:

- gol anulado;
- cartão corrigido;
- minuto corrigido;
- estatística recalculada;
- evento removido;
- substituição corrigida.

O reducer precisa aceitar `REVISION` ou reconstruir o estado a partir do log.

---

## 8. Arquitetura GCP de baixo custo

### 8.1 Diagrama

```text
                    +---------------------------+
                    | Provider de dados ao vivo |
                    +-------------+-------------+
                                  |
                                  | polling/API
                                  v
+----------------------------------------------------------------+
| Compute Engine e2-micro - us-central1                           |
|                                                                |
|  golo-core (Go)                                            |
|  + collector                                                   |
|  + normalizer                                                  |
|  + event log                                                   |
|  + match reducer                                               |
|  + rolling feature engine                                      |
|  + probability engine                                          |
|  + replay engine                                                |
|  + publisher Firebase                                           |
|                                                                |
|  SQLite WAL + arquivos temporários                              |
+-------------------+----------------------+---------------------+
                    |                      |
                    | snapshots ao vivo    | arquivos compactados
                    v                      v
        +------------------------+   +---------------------------+
        | Firebase Realtime DB  |   | Cloud Storage            |
        | latest state/predict. |   | raw + parquet/jsonl.gz   |
        +-----------+------------+   +-------------+-------------+
                    |                              |
                    | realtime subscription        | carga em lote
                    v                              v
        +------------------------+       +------------------------+
        | React/Vite SPA         |       | BigQuery               |
        | Firebase Hosting       |       | avaliação e pesquisa   |
        +------------------------+       +------------------------+

Treinamento: Python local/Colab -> model artifact JSON -> VM
```

### 8.2 Por que uma VM e não somente Cloud Run

O coletor precisa permanecer ativo e consultar providers com frequência durante jogos. Cloud Run com cobrança por requisição é excelente para APIs intermitentes, mas não é a opção mais simples para um loop contínuo sem requisição. Uma instância mínima ou worker contínuo pode gerar custo maior que a VM gratuita.

A `e2-micro` oferece:

- processo contínuo;
- cron local;
- SQLite persistente;
- debugging simples;
- implantação por um binário Go;
- computação elegível ao Free Tier em regiões específicas dos EUA.

### 8.3 Região

Usar `us-central1` por:

- elegibilidade da `e2-micro` gratuita;
- Cloud Storage gratuito em regiões dos EUA;
- proximidade lógica com outros serviços GCP gratuitos;
- ampla disponibilidade de produtos.

O atraso adicional até o Brasil tende a ser aceitável para o dashboard, mas o fator crítico será a latência entre o provider e a VM, não apenas entre a VM e o navegador.

### 8.4 Componentes GCP

| Serviço | Uso no MVP | Configuração |
|---|---|---|
| Compute Engine | processo contínuo | 1 `e2-micro`, `us-central1`, 30 GB standard persistent disk |
| Firebase Hosting | frontend | SPA estática, sem SSR |
| Realtime Database | estado ao vivo | apenas estado recente e previsão atual |
| Cloud Storage | raw e datasets | bucket regional nos EUA, lifecycle de retenção |
| BigQuery | avaliação | tabelas particionadas e consultas selecionando colunas |
| Artifact Registry | imagem/binário opcional | manter menos de 500 MB |
| Cloud Monitoring | health e métricas | métricas essenciais; evitar logs excessivos |
| Budget Alerts | proteção financeira | alertas em 50%, 80% e 100% do orçamento |

### 8.5 O que não usar no MVP

- Cloud SQL;
- GKE;
- Managed Kafka;
- Dataflow;
- Vertex AI Endpoint sempre ativo;
- Cloud Load Balancer;
- Memorystore;
- múltiplas VMs;
- Pub/Sub como requisito;
- microserviços separados.

### 8.6 Evolução sem reescrever o domínio

| Pressão | Evolução |
|---|---|
| Mais usuários no frontend | manter Firebase; aumentar plano conforme consumo |
| Mais partidas | mover collectors para Cloud Run Jobs ou VMs por grupo de ligas |
| Mais eventos | introduzir Pub/Sub entre normalizer e consumers |
| SQLite insuficiente | PostgreSQL/Cloud SQL ou AlloyDB somente com receita |
| Analytics pesado | ClickHouse ou BigQuery otimizado |
| Modelos pesados | serviço Python em Cloud Run com scale-to-zero |
| Alta disponibilidade | dois workers com lease distribuído e idempotência |

---

## 9. Componentes internos do monólito

### 9.1 `collector`

Responsabilidades:

- descobrir partidas ao vivo;
- respeitar rate limits;
- controlar cursor por partida;
- retries com jitter;
- circuit breaker;
- medir latência e erro;
- armazenar payload original antes da transformação.

### 9.2 `normalizer`

- mapear IDs do provider para IDs internos;
- converter eventos ao esquema canônico;
- corrigir timezone;
- deduplicar;
- marcar evento atrasado;
- preservar dados não reconhecidos em `attributes`.

### 9.3 `eventstore`

No MVP, implementado sobre SQLite:

- append-only sempre que possível;
- transação por lote de eventos;
- índice por `match_id, match_second`;
- índice único por `event_id`;
- retenção local limitada;
- exportação diária para `jsonl.gz` ou Parquet.

### 9.4 `reducer`

Função pura conceitual:

```text
new_state = reduce(previous_state, event)
```

Nunca depender do frontend ou do provider original.

### 9.5 `featureengine`

- atualiza acumuladores incrementais;
- mantém ring buffers por partida;
- produz snapshot a cada evento importante ou a cada intervalo;
- registra `feature_schema_version`;
- usa apenas dados com timestamp menor ou igual ao instante de corte.

### 9.6 `predictor`

- carrega artefato versionado;
- calcula probabilidades brutas;
- aplica calibração;
- produz fallback do baseline;
- rejeita input incompatível;
- inclui versão e hash do artefato na saída.

### 9.7 `quality`

Calcula `data_quality_score` com base em:

- atraso do feed;
- campos ausentes;
- relógio parado;
- divergência entre snapshot e eventos;
- frequência de atualização;
- correções recentes;
- health do provider.

### 9.8 `publisher`

- publica somente quando a previsão muda de forma material ou atinge o intervalo máximo;
- usa escrita idempotente no Firebase;
- evita publicar raw payload;
- remove partidas finalizadas após período configurado.

### 9.9 `replay`

- lê eventos armazenados;
- respeita ordem de `event_time` e regras de chegada;
- permite simular atraso real;
- compara output com snapshots de referência;
- é o principal instrumento de teste end-to-end.

---

## 10. Contratos de estado e previsão

### 10.1 Estado da partida

```json
{
  "matchId": "br-serie-a-2026-123",
  "status": "LIVE",
  "period": 2,
  "clockSeconds": 4270,
  "score": { "home": 1, "away": 0 },
  "redCards": { "home": 0, "away": 0 },
  "shotsLast5m": { "home": 4, "away": 1 },
  "shotsOnTargetLast5m": { "home": 2, "away": 0 },
  "xgLast5m": { "home": 0.48, "away": 0.03 },
  "provider": "provider-a",
  "providerUpdatedAt": "2026-07-25T20:42:30Z",
  "receivedAt": "2026-07-25T20:42:31Z",
  "feedLagMs": 1000,
  "stateVersion": 192
}
```

### 10.2 Previsão

```json
{
  "matchId": "br-serie-a-2026-123",
  "asOfMatchSecond": 4270,
  "calculatedAt": "2026-07-25T20:42:31.250Z",
  "probabilities": {
    "goalNext5m": 0.148,
    "goalNext10m": 0.272,
    "goalBeforeFullTime": 0.613,
    "homeGoalBeforeFullTime": 0.441,
    "awayGoalBeforeFullTime": 0.307
  },
  "dataQuality": 0.91,
  "confidenceBand": "HIGH",
  "modelVersion": "goal-hazard-001",
  "calibratorVersion": "platt-001",
  "featureVersion": "live-features-001",
  "predictionSequence": 192
}
```

### 10.3 Paths no Realtime Database

```text
/public/live-matches/{matchId}/meta
/public/live-matches/{matchId}/state
/public/live-matches/{matchId}/prediction
/public/live-matches/{matchId}/recent-events
/public/live-index/{competitionId}/{matchId}
/system/provider-health/{provider}
```

Não armazenar todo o histórico no Realtime Database. O histórico pertence ao SQLite, Cloud Storage e BigQuery.

---

## 11. Modelo de dados SQLite

```sql
CREATE TABLE matches (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_match_id TEXT NOT NULL,
    competition_id TEXT NOT NULL,
    season_id TEXT,
    home_team_id TEXT NOT NULL,
    away_team_id TEXT NOT NULL,
    scheduled_at TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(provider, provider_match_id)
);

CREATE TABLE match_events (
    event_id TEXT PRIMARY KEY,
    match_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_event_id TEXT,
    event_type TEXT NOT NULL,
    team_id TEXT,
    player_id TEXT,
    match_second INTEGER NOT NULL,
    period INTEGER NOT NULL,
    provider_time TEXT,
    received_at TEXT NOT NULL,
    processed_at TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    FOREIGN KEY(match_id) REFERENCES matches(id)
);

CREATE INDEX idx_events_match_time
ON match_events(match_id, match_second, received_at);

CREATE TABLE feature_snapshots (
    snapshot_id TEXT PRIMARY KEY,
    match_id TEXT NOT NULL,
    match_second INTEGER NOT NULL,
    cutoff_time TEXT NOT NULL,
    features_json TEXT NOT NULL,
    feature_version TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_features_match_time
ON feature_snapshots(match_id, match_second);

CREATE TABLE predictions (
    prediction_id TEXT PRIMARY KEY,
    match_id TEXT NOT NULL,
    snapshot_id TEXT NOT NULL,
    horizon_seconds INTEGER NOT NULL,
    probability_raw REAL NOT NULL,
    probability_calibrated REAL NOT NULL,
    data_quality REAL NOT NULL,
    model_version TEXT NOT NULL,
    calibrator_version TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_predictions_match_horizon
ON predictions(match_id, horizon_seconds, created_at);

CREATE TABLE provider_cursors (
    provider TEXT NOT NULL,
    provider_match_id TEXT NOT NULL,
    cursor TEXT,
    last_success_at TEXT,
    last_error_at TEXT,
    error_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(provider, provider_match_id)
);
```

---

## 12. Pipeline de treinamento e backtest

### 12.1 Dataset de snapshots

O dataset não é uma linha por partida. É uma linha por **partida, instante e horizonte**.

Exemplo:

```text
match_id | second | score_diff | shots_5m | xg_5m | red_diff | horizon | label
A        | 3600   | 0          | 3        | 0.31  | 0        | 300     | 1
A        | 3660   | 0          | 2        | 0.18  | 0        | 300     | 0
```

### 12.2 Split correto

Nunca usar split aleatório de snapshots. Snapshots da mesma partida são altamente correlacionados.

Usar:

- split por partida;
- preferencialmente split temporal por temporada ou data;
- competição futura ou período futuro como teste final;
- nenhuma partida do teste presente no treino ou calibração.

### 12.3 Prevenção de leakage

Proibido usar:

- estatística final da partida;
- evento com `received_at` posterior ao cutoff;
- odds corrigidas depois do gol;
- xG recalculado posteriormente sem timestamp original;
- escalação confirmada depois do snapshot;
- agregados de temporada que incluam a própria partida futura.

### 12.4 Pipeline

```text
raw events
    -> canonical events
    -> replay em ordem temporal
    -> feature snapshots
    -> labels por horizonte
    -> train/validation/calibration/test split
    -> baseline
    -> candidate model
    -> calibration
    -> report
    -> artifact JSON
```

### 12.5 Artefato do modelo

O runtime Go deve carregar um artefato simples:

```json
{
  "modelVersion": "goal-hazard-001",
  "featureVersion": "live-features-001",
  "horizonSeconds": 600,
  "intercept": -2.143,
  "coefficients": {
    "remaining_minutes": 0.031,
    "score_is_draw": 0.182,
    "losing_team_pressure_5m": 0.217,
    "shots_on_target_5m": 0.164,
    "xg_5m": 0.841,
    "red_card_difference": -0.302
  },
  "normalization": {},
  "calibration": {
    "type": "platt",
    "a": 0.94,
    "b": -0.07
  },
  "trainedUntil": "2026-06-30",
  "sha256": "..."
}
```

### 12.6 Métricas obrigatórias

#### Probabilidade

- Brier Score;
- Log Loss;
- calibration slope e intercept;
- Expected Calibration Error;
- reliability diagram.

#### Discriminação

- ROC AUC;
- PR AUC, especialmente em horizontes curtos;
- lift por decil.

#### Operação

- latência total;
- percentual de snapshots com feed stale;
- falhas por provider;
- divergência replay versus live;
- uso de CPU e memória.

#### Negócio, somente depois

- closing-line value;
- edge após remoção da margem;
- retorno simulado com regras congeladas;
- drawdown;
- volume de sinais.

ROI nunca deve ser a única métrica de seleção de modelo.

### 12.7 Baselines obrigatórios

Comparar qualquer modelo com:

1. taxa média da liga por minuto;
2. Poisson pré-jogo sem features ao vivo;
3. probabilidade do mercado sem margem, quando disponível;
4. último valor conhecido sem atualização;
5. modelo que usa apenas minuto e placar.

---

## 13. Decision engine

O predictor gera probabilidades. Um componente separado decide se um **cenário salvo pelo usuário** está ocorrendo e merece notificação.

A diferença em relação à versão 1.0 é de autoria: o Golo não decide mais que algo é uma boa entrada. **O usuário define o cenário; o Golo reconhece que ele está acontecendo e relata o que a história diz a respeito.** Isso não é uma sutileza de linguagem — muda quem é responsável pela tese e o que o produto precisa provar para funcionar.

### 13.1 Regra de notificação

```text
notificar o dono do cenário se:
- o cenário está validado com amostra suficiente
- data_quality >= 0.85
- feed_lag <= limite da liga/provider
- o gatilho do cenário casa com o estado atual da partida
- não houve gol nos últimos N segundos (o estado acabou de mudar)
- não houve notificação para o mesmo cenário e partida
```

**Gate adicional sobre a evidência.** Um cenário só é armado se o limite inferior do intervalo superar o ponto de equilíbrio da odd alvo. Sem isso, não arma.

O motivo é direto: um alerta sobre um cenário não validado é um palpite com carimbo de autoridade — **pior do que nenhum alerta**, porque empresta credibilidade a ruído.

### 13.1.1 Forma da mensagem

Obrigatório em toda notificação:

- qual cenário disparou, com o nome que **o usuário** deu;
- partida e minuto;
- taxa histórica **com intervalo e número de partidas**;
- odd mínima derivada do limite pessimista;
- rodapé legal (§23).

Proibido:

- verbo imperativo de aposta ("aposte", "entre", "aproveite");
- linguagem de urgência ou escassez;
- qualquer projeção de lucro;
- omitir o intervalo ou o tamanho da amostra.

### 13.2 Value contra mercado

Para odds decimais `o_i`, a probabilidade implícita bruta é:

```text
q_i = 1 / o_i
```

Para um mercado com resultados mutuamente exclusivos, remover a margem por normalização simples:

```text
p_i_market = q_i / soma(q_j)
```

No mercado de "gol até o fim: sim/não", comparar:

```text
edge = p_model - p_market
```

Não usar edge sem:

- odds timestampadas;
- remoção da margem;
- calibração do modelo;
- custo de execução considerado;
- política de suspensão após evento relevante.

### 13.3 Rótulo binário

Se a UI exigir `SIM` ou `NÃO`:

```text
SIM: p >= threshold_contextual
NÃO: p < threshold_contextual
```

O threshold não deve ser fixado automaticamente em 50%. Ele depende de horizonte, custo e objetivo.

---

## 14. Frontend e experiência de uso

### 14.1 Stack

- React;
- TypeScript;
- Vite;
- Tailwind CSS;
- TanStack Query apenas para chamadas não realtime;
- Firebase SDK para Realtime Database;
- ECharts ou Recharts para a série temporal;
- Zod para validar payloads.

### 14.2 Wireframe

```text
+-------------------------------------------------------------+
| GOLO                         LIVE | MODELO 001 | FEED OK |
+-------------------------------------------------------------+
| Palmeiras 1 x 0 Flamengo                         71:10       |
| Ultima atualizacao: 0,9s | Qualidade: 91%                    |
+-------------------------------------------------------------+
| GOL ATE O FIM                                               |
|                         61,3%                               |
| Proximos 5 min: 14,8% | Proximos 10 min: 27,2%             |
+-------------------------------------------------------------+
| Mandante marca: 44,1% | Visitante marca: 30,7%              |
+-------------------------------------------------------------+
| Grafico: probabilidade ao longo da partida                   |
+-------------------------------------------------------------+
| Fatores recentes                                             |
| + xG mandante ultimos 5m                                     |
| + 2 chutes no alvo                                           |
| - vantagem atual no placar                                   |
+-------------------------------------------------------------+
| Aviso: probabilidades sao estimativas, nao garantias.        |
+-------------------------------------------------------------+
```

### 14.3 Regras de UX

- sempre mostrar o horizonte;
- sempre mostrar quando foi atualizado;
- nunca usar linguagem como "certeza", "aposta garantida" ou "dinheiro fácil";
- degradar visualmente previsões com feed atrasado;
- não esconder falhas;
- mostrar probabilidade, não apenas cor verde/vermelha;
- usar gráfico com escala fixa de 0% a 100%;
- evitar animações que incentivem impulso;
- diferenciar claramente previsão e resultado real.

### 14.4 Autenticação

Para o MVP privado:

- opção mais simples: Firebase Hosting público com URL não divulgada e sem dados sensíveis;
- opção recomendada: Firebase Authentication com Google Sign-In e allowlist de e-mails.

Não colocar credenciais do provider no frontend.

---

## 15. Estrutura do repositório

```text
golo/
|-- apps/
|   `-- web/                         # React/Vite
|       |-- src/
|       |-- firebase.json
|       `-- vite.config.ts
|-- cmd/
|   `-- golo/
|       `-- main.go
|-- internal/
|   |-- config/
|   |-- domain/
|   |-- providers/
|   |   |-- provider_a/
|   |   `-- replay/
|   |-- collector/
|   |-- normalizer/
|   |-- eventstore/
|   |-- reducer/
|   |-- features/
|   |-- predictor/
|   |-- calibration/
|   |-- quality/
|   |-- publisher/
|   `-- observability/
|-- ml/
|   |-- notebooks/
|   |-- src/
|   |   |-- build_dataset.py
|   |   |-- train_baseline.py
|   |   |-- calibrate.py
|   |   `-- evaluate.py
|   |-- reports/
|   `-- requirements.txt
|-- models/
|   |-- goal-hazard-001.json
|   `-- manifest.json
|-- schemas/
|   |-- match-event.schema.json
|   |-- match-state.schema.json
|   `-- prediction.schema.json
|-- migrations/
|-- scripts/
|   |-- deploy-vm.sh
|   |-- export-daily.sh
|   `-- replay-match.sh
|-- infrastructure/
|   |-- terraform/
|   |-- systemd/
|   `-- firebase/
|-- tests/
|   |-- fixtures/
|   |-- integration/
|   `-- replay/
|-- Makefile
|-- docker-compose.yml
|-- .env.example
`-- README.md
```

### 15.1 Regra de dependências

```text
providers -> domain
collector -> providers + eventstore
normalizer -> domain
reducer -> domain
features -> domain
predictor -> features
publisher -> prediction DTO
```

O domínio não importa Firebase, SQLite ou SDK de provider.

---

## 16. Configuração

### 16.1 `.env.example`

```bash
APP_ENV=development
LOG_LEVEL=info

PROVIDER_NAME=provider_a
PROVIDER_API_KEY=
PROVIDER_POLL_INTERVAL_SECONDS=10
PROVIDER_REQUEST_TIMEOUT_SECONDS=5

SQLITE_PATH=./data/golo.db
MODEL_PATH=./models/goal-hazard-001.json

FIREBASE_PROJECT_ID=
FIREBASE_DATABASE_URL=
GOOGLE_APPLICATION_CREDENTIALS=

GCS_BUCKET_RAW=
BIGQUERY_DATASET=golo_analytics

PUBLISH_MIN_INTERVAL_SECONDS=5
PUBLISH_MAX_INTERVAL_SECONDS=20
STALE_FEED_THRESHOLD_SECONDS=30
```

### 16.2 Config por competição

```yaml
competitions:
  br-serie-a:
    enabled: true
    provider_competition_id: "123"
    poll_interval_seconds: 8
    stale_threshold_seconds: 25
    model_version: goal-hazard-001
    minimum_data_quality: 0.85
```

---

## 17. Deploy e operação

### 17.1 Provisionamento inicial

1. Criar projeto GCP exclusivo.
2. Vincular conta de faturamento com os créditos corretos.
3. Criar budget de baixo valor e alertas.
4. Habilitar Compute Engine, Firebase, Cloud Storage e BigQuery.
5. Criar VM `e2-micro` em `us-central1`.
6. Usar service account com permissões mínimas.
7. Criar bucket regional nos EUA.
8. Inicializar Firebase Hosting e Realtime Database.
9. Implantar binário Go via `systemd`.
10. Publicar frontend estático.

### 17.2 Processo `systemd`

```ini
[Unit]
Description=Golo Core
After=network-online.target

[Service]
User=golo
WorkingDirectory=/opt/golo
ExecStart=/opt/golo/bin/golo
Restart=always
RestartSec=5
EnvironmentFile=/opt/golo/.env
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

### 17.3 Backup

- snapshot diário do SQLite com API de backup, não simples cópia durante escrita;
- compressão e upload para Cloud Storage;
- lifecycle: raw detalhado por prazo curto, agregados por prazo maior;
- teste mensal de restauração;
- artefatos de modelo imutáveis.

### 17.4 CI/CD

GitHub Actions:

```text
pull request:
- go test ./...
- go vet ./...
- staticcheck
- frontend lint/test/build
- schema validation
- replay golden tests

main:
- build binario linux amd64
- build frontend
- upload release artifact
- deploy frontend Firebase
- copiar binario para VM
- restart systemd
- health check
```

O deploy precisa permitir rollback para o binário e modelo anteriores.

---

## 18. Testes

### 18.1 Unitários

- normalização por tipo de evento;
- deduplicação;
- reducer;
- rolling windows;
- cálculo de Poisson/hazard;
- calibração;
- score de qualidade;
- serialização dos contratos.

### 18.2 Golden replay tests

Para partidas fixas:

- input: lista imutável de eventos;
- output esperado: estados e previsões em instantes definidos;
- mudança no output exige atualização consciente do golden file.

### 18.3 Property tests

Invariantes:

- probabilidade entre 0 e 1;
- placar nunca diminui sem evento de correção;
- `processed_at >= received_at`;
- janela não contém evento futuro;
- replay é determinístico;
- duplicar um evento não muda o estado final;
- gol anulado restaura o estado correto.

### 18.4 Integração

- provider fake;
- SQLite temporário;
- Firebase Emulator Suite;
- exportação para arquivo;
- carga de modelo incompatível deve falhar com erro claro.

### 18.5 Shadow mode

Modelos candidatos calculam previsões, mas não alimentam o valor principal do frontend. Depois do resultado, comparar `champion` e `candidate` no mesmo conjunto de snapshots.

---

## 19. Observabilidade

### 19.1 Métricas

- `provider_requests_total`;
- `provider_errors_total`;
- `provider_feed_lag_seconds`;
- `events_received_total`;
- `events_deduplicated_total`;
- `active_matches`;
- `prediction_latency_ms`;
- `prediction_publish_total`;
- `prediction_probability` por horizonte, com cuidado de cardinalidade;
- `sqlite_size_bytes`;
- `process_memory_bytes`;
- `replay_divergence_total`.

### 19.2 Logs

Estruturados em JSON:

```json
{
  "severity": "INFO",
  "message": "prediction_calculated",
  "matchId": "...",
  "modelVersion": "goal-hazard-001",
  "probability": 0.613,
  "feedLagMs": 820,
  "durationMs": 4
}
```

Não logar payload completo repetidamente no Cloud Logging. Armazenar raw compactado no bucket para evitar custo e ruído.

### 19.3 Alertas

- nenhum evento de partida ao vivo por tempo excessivo;
- feed lag acima do threshold;
- taxa de erro do provider;
- disco acima de 75%;
- processo reiniciando repetidamente;
- publicação Firebase falhando;
- gasto GCP acima do orçamento.

---

## 20. Segurança

### 20.1 IAM

Service account da VM com somente:

- escrever no bucket específico;
- escrever nos paths necessários do Firebase;
- inserir jobs/dados necessários no dataset analítico;
- enviar métricas/logs essenciais.

Evitar papel `Editor` do projeto.

### 20.2 Segredos

No primeiro deploy:

- chave do provider em arquivo de ambiente protegido `0600`;
- acesso somente pelo usuário do serviço;
- não versionar `.env`;
- rotacionar chave após exposição.

Evolução recomendada: Secret Manager quando o projeto justificar.

### 20.3 Firebase Rules

- leitura pública somente se o MVP realmente for público;
- escrita exclusiva para Admin SDK/service account;
- admin protegido por autenticação e allowlist;
- validar shape e limites de payload.

### 20.4 Superfície pública

O core da VM não precisa expor uma API pública. Ele precisa apenas:

- fazer conexões de saída para o provider;
- publicar no Firebase;
- enviar backups ao Storage.

SSH deve ser restrito por IAP ou firewall ao IP necessário, sem senha.

---

## 21. Estimativa de custo

### 21.1 Infraestrutura GCP

Valores de referência em 25 de julho de 2026. Preços e níveis gratuitos podem mudar.

| Item | Uso planejado | Estimativa mensal |
|---|---|---:|
| Compute Engine `e2-micro` | 1 VM 24/7 em região elegível | US$ 0 de compute dentro do Free Tier |
| IPv4 externo | aproximadamente 730 h x US$ 0,005 | aproximadamente US$ 3,65 |
| Disco standard | até 30 GB-mês | US$ 0 dentro do Free Tier |
| Firebase Hosting | SPA pequena | US$ 0 dentro das cotas |
| Realtime Database | menos de 100 conexões, até 1 GB | US$ 0 no Spark, se dentro das cotas |
| Cloud Storage | arquivos compactados abaixo de 5 GB-mês em região elegível | US$ 0 dentro do Free Tier |
| BigQuery | até 10 GB e consultas abaixo de 1 TiB/mês | US$ 0 dentro do Free Tier |
| Artifact Registry | menos de 0,5 GB | US$ 0 dentro do Free Tier |
| Logging/Monitoring | logs amostrados | esperado US$ 0, sujeito a volume |
| Total GCP provável | sem provider externo | aproximadamente US$ 3,65 a US$ 7 |

### 21.2 Crédito Google AI Pro

O Google Developer Program Premium incluído no Google AI Pro oferece **US$ 10 de crédito mensal para GenAI e Google Cloud**, após vinculação correta ao perfil e à conta de faturamento. Esse valor provavelmente cobre o MVP GCP descrito.

O saldo chamado de "créditos de IA" em produtos Google One pode ser destinado a ferramentas como Flow e Antigravity. Não considerar um saldo de R$ 60 exibido no chat/app Gemini como crédito de API ou GCP até ele aparecer explicitamente no Cloud Billing ou AI Studio.

### 21.3 Custo não-GCP mais importante

O feed ao vivo pode ser o maior custo real.

| Estratégia | Custo | Risco |
|---|---:|---|
| dados abertos históricos | zero | não resolve live completo |
| trial de API | zero temporário | prazo curto e cobertura limitada |
| plano inicial de provider | por exemplo, a partir de EUR 29/mês | custo superior à infraestrutura |
| scraping | zero financeiro direto | instabilidade, atraso, bloqueio e termos de uso |

### 21.4 Proteções financeiras

- orçamento GCP inicial: US$ 10/mês;
- alertas: 50%, 80%, 100%;
- quotas restritas;
- uma única conta/projeto;
- sem serviços com mínimo de instância;
- revisar Cloud Logging semanalmente;
- lifecycle no Storage;
- limitar consultas BigQuery com `maximum_bytes_billed`;
- desligar ambiente quando não houver necessidade de monitoramento.

---

## 22. Riscos e mitigação

| Risco | Impacto | Mitigação MVP |
|---|---|---|
| feed atrasado | previsão reage tarde | medir lag, reduzir confiança e bloquear sinal |
| provider muda schema | interrupção | contract tests e adapter isolado |
| eventos corrigidos | estado incorreto | event log e replay determinístico |
| poucos dados | overfitting | baseline simples, regularização e split temporal |
| leakage | métricas falsas | cutoff rigoroso por event/received time |
| baixa calibração | probabilidades enganosas | calibrador fora da amostra e reliability plots |
| liga heterogênea | modelo instável | começar com poucas ligas e efeitos hierárquicos depois |
| SQLite corrompido | perda operacional | WAL, backup consistente e restore test |
| limite de memória da VM | restart | binário Go, buffers limitados e profiling |
| custo de provider | inviabilidade comercial | validar sinal antes de contratar cobertura ampla |
| mercado eficiente | nenhum edge sustentável | comparar com mercado e aceitar resultado negativo |
| scraping bloqueado | perda do feed | provider oficial como caminho recomendado |
| comportamento compulsivo | dano ao usuário | UX sem urgência, avisos, limites e acesso 18+ |
| mudança regulatória | retrabalho | revisão jurídica antes de monetização/afiliados |

---

## 23. Jogo responsável e limite regulatório

Este blueprint assume uma **ferramenta de medição**, sem custódia de dinheiro, sem execução de apostas e sem recomendação de entrada.

O reposicionamento da versão 2.0 melhora a posição jurídica, e vale ser explícito sobre por quê: **vender uma régua é diferente de vender uma promessa.** O Golo afirma "este cenário ocorreu 1.356 vezes e resultou em gol 50,4% delas, com intervalo de 47,8% a 53,1%" — um fato histórico verificável. Não afirma que vai se repetir, não recomenda entrada e não projeta retorno.

Isso **não** dispensa revisão jurídica. Ferramenta vendida a apostadores continua alcançada pelas regras de publicidade ampliadas em julho de 2026, que responsabilizam quem divulga. A posição é mais defensável, não isenta.

No Brasil, somente operadores autorizados pela Secretaria de Prêmios e Apostas podem ofertar apostas de quota fixa nacionalmente. Desde 1 de janeiro de 2025, operadores federais autorizados usam domínio `.bet.br`.

Em julho de 2026 foram ampliadas as exigências de publicidade de apostas, incluindo advertências obrigatórias e responsabilidade dos envolvidos na divulgação. Portanto, antes de adicionar afiliados, links de casas, publicidade ou linguagem promocional, é necessária revisão jurídica específica.

Princípios do produto:

- acesso somente para maiores de 18 anos em eventual versão pública;
- "aposta não é investimento";
- nenhuma promessa de lucro;
- nenhuma recuperação de perda;
- nenhuma notificação de urgência;
- permitir ocultar ou desativar sinais;
- oferecer informação sobre jogo responsável;
- evitar gamificação de sequência de ganhos;
- deixar claro que probabilidades podem estar erradas.

---

## 24. Roadmap por gates

### Gate 0 - Fundação de dados

Entregáveis:

- esquema canônico;
- provider fake;
- importador de dados históricos;
- reducer;
- SQLite;
- replay determinístico;
- golden tests.

Critério de saída:

- uma partida histórica é reproduzida do início ao fim com estado consistente.

### Gate 1 - Baseline offline

Entregáveis:

- geração de snapshots;
- labels por horizonte;
- split temporal;
- baseline Poisson/hazard;
- relatório de Brier, Log Loss e calibração;
- artefato JSON carregado pelo Go.

Critério de saída:

- o pipeline é reproduzível e o modelo supera pelo menos um baseline ingênuo no teste.

### Gate 2 - Piloto ao vivo

Entregáveis:

- adapter do provider real;
- coleta de uma competição;
- score de qualidade;
- publicação Firebase;
- dashboard ao vivo;
- persistência e backup.

Critério de saída:

- partidas completas são monitoradas sem intervenção manual e com lag visível.

### Gate 3 - MVP privado

Entregáveis:

- múltiplas partidas;
- tela de avaliação;
- replay pela UI;
- autenticação opcional;
- alertas operacionais;
- documentação de incidentes;
- budget alerts.

Critério de saída:

- o sistema opera de forma confiável durante uma amostra suficiente de partidas futuras.

### Gate 4 - Modelo candidato

Entregáveis:

- LightGBM ou outro modelo tabular;
- shadow mode;
- calibração;
- comparação com champion;
- segmentação por liga e minuto.

Critério de saída:

- candidato melhora métricas fora da amostra sem piorar materialmente a calibração.

### Gate 5 - Decisão de produto

Possíveis decisões:

- encerrar por ausência de sinal;
- manter como ferramenta privada;
- vender API/widget B2B;
- criar produto informativo B2C;
- integrar dados de mercado;
- contratar feed profissional.

---

## 25. Backlog priorizado

### P0 - indispensável

1. Criar monorepo e contratos JSON Schema.
2. Implementar provider fake e replay.
3. Implementar evento canônico e deduplicação.
4. Implementar SQLite migrations.
5. Implementar reducer e ring buffers.
6. Criar snapshots e labels sem leakage.
7. Treinar baseline.
8. Exportar artefato JSON.
9. Implementar predictor Go.
10. Criar golden replay tests.
11. Integrar provider ao vivo.
12. Publicar latest state no Firebase.
13. Criar Live Board.
14. Criar tela de partida.
15. Configurar VM, backup e budget.

### P1 - depois do MVP funcional

- painel de avaliação;
- odds e remoção da margem;
- modelo candidato LightGBM;
- explicabilidade;
- autenticação;
- múltiplos providers;
- alertas internos;
- exportação automatizada para BigQuery.

### P2 - somente com evidência

- Pub/Sub;
- API pública;
- widget white-label;
- planos pagos;
- mobile/PWA;
- personalização;
- notificações;
- múltiplas regiões;
- alta disponibilidade.

---

## 26. Primeira sequência de implementação

A ordem recomendada é:

1. **Repositório e domínio** - criar tipos, schemas e regras de tempo.
2. **Replay antes do live** - construir provider de arquivos e reducer.
3. **Dataset de snapshots** - provar que os labels são corretos.
4. **Baseline matemático** - produzir probabilidades sem UI.
5. **Relatório de avaliação** - rejeitar modelo ruim cedo.
6. **Artefato Go** - garantir paridade Python versus Go.
7. **Provider ao vivo** - integrar uma competição e registrar raw.
8. **Firebase** - publicar somente latest state/prediction.
9. **Frontend** - mostrar estado, probabilidades e qualidade.
10. **Operação GCP** - deploy, backup, alertas e orçamento.

A UI não deve ser a primeira parte. O primeiro produto real é o replay reproduzível.

---

## 27. Definition of Done do MVP

### Produto

- [ ] uma pessoa consegue abrir a aplicação e encontrar partidas monitoradas;
- [ ] cada partida mostra probabilidade e horizonte;
- [ ] feed stale é claramente indicado;
- [ ] gráfico histórico funciona;
- [ ] existe aviso de caráter probabilístico e jogo responsável.

### Dados

- [ ] raw payload é preservado;
- [ ] eventos possuem os quatro tempos;
- [ ] deduplicação é idempotente;
- [ ] correções podem reconstruir o estado;
- [ ] replay é determinístico.

### Modelo

- [ ] treino possui split por partida e tempo;
- [ ] existe baseline;
- [ ] probabilidade é calibrada fora da amostra;
- [ ] relatório contém Brier e Log Loss;
- [ ] modelo e features são versionados;
- [ ] Python e Go geram o mesmo resultado dentro de tolerância.

### Infraestrutura

- [ ] VM é `e2-micro` em região elegível;
- [ ] budget alerts configurados;
- [ ] credenciais não estão no repositório;
- [ ] backup é testado;
- [ ] processo reinicia automaticamente;
- [ ] frontend não recebe chave do provider.

### Operação

- [ ] health do provider é visível;
- [ ] latência é medida;
- [ ] incidentes são registrados;
- [ ] existe rollback de binário e modelo;
- [ ] custos são revisados.

---

## 28. Registro de decisões arquiteturais

### ADR-001 - Monólito modular

**Decisão:** um processo Go contém collector, reducer, features, predictor e publisher.

**Motivo:** custo, simplicidade operacional e volume inicial.

### ADR-002 - SQLite antes de Cloud SQL

**Decisão:** SQLite WAL na VM.

**Motivo:** uma única instância escritora e necessidade de replay local. Cloud SQL adicionaria custo fixo desnecessário.

### ADR-003 - Firebase Realtime Database para live state

**Decisão:** RTDB distribui somente estado e previsão recentes.

**Motivo:** assinatura realtime pronta, frontend simples e cota gratuita adequada ao MVP privado.

### ADR-004 - Treinamento Python, inferência Go

**Decisão:** modelos simples exportados como JSON.

**Motivo:** ecossistema científico no treino e runtime de baixa memória em produção.

### ADR-005 - LLM fora do prediction loop

**Decisão:** Gemini pode auxiliar desenvolvimento e documentação, mas não decide a probabilidade.

**Motivo:** custo, não determinismo, calibração inadequada e ausência de vantagem comprovada para o alvo numérico.

### ADR-006 - Provider substituível

**Decisão:** adapter obrigatório.

**Motivo:** preço, cobertura e estabilidade do feed são variáveis e representam o maior risco do projeto.

### ADR-007 - Sem aposta automática

**Decisão:** sinais são somente analíticos e em modo sombra.

**Motivo:** validar ciência e operação antes de assumir risco financeiro, jurídico e comportamental.

### ADR-008 - O produto é a régua, não a previsão

**Data:** 26 de julho de 2026
**Status:** aceito, substitui o posicionamento da versão 1.0

**Decisão:** o Golo passa a ser vendido como ferramenta de validação de cenários. A previsão ao vivo continua existindo como infraestrutura e como leitura de jogo, mas deixa de ser a proposta de valor.

**Motivo — o que foi medido.** Sobre 3.287 partidas com 80.261 chutes datados, o modelo empata com uma taxa constante em todos os horizontes. A única "vitória" (Brier de gol até o fim, `0,15036` contra `0,15042`) tem intervalo de 95% agrupado por partida de `[-0,001025, +0,000853]` — atravessa o zero. Não é vantagem, é ruído.

**Por que dado melhor não resolve.** Comprar xG elevaria a precisão absoluta, mas as casas de apostas operam com o mesmo dado ou melhor. Vantagem vem de possuir informação que o outro lado não possui, não de possuir informação boa. Nenhuma compra ao alcance deste projeto cria essa assimetria.

**O que a medição revelou de valioso.** O aparato construído para provar a vantagem — intervalos de Wilson, agrupamento por partida, uma observação por partida, testes selados com contrato imutável, recusa em armar sem evidência — é rigoroso de um jeito que quase nenhum produto do setor é. Essa peça não compete com Opta nem com bet365, porque eles não vendem honestidade sobre a própria incerteza.

**Consequências aceitas:**

- O mercado de dicas fica fora de alcance, e isso é definitivo: esse público quer confiança, não calibração;
- O sucesso do produto deixa de depender de vencer o mercado;
- Um veredito "não há vantagem aqui" passa a ser entrega, não fracasso;
- O bot de Telegram sobrevive inteiro, com autoria trocada — ele relata cenários **do usuário**, não teses do Golo.

**O que teria mudado a decisão:** um intervalo de confiança inteiramente abaixo de zero em qualquer horizonte. Não houve.

### ADR-009 - Qualificação exige significância, não comparação

**Data:** 26 de julho de 2026
**Status:** aceito

**Decisão:** nenhuma estratégia é armada com base em comparação de estimativas pontuais. O intervalo de confiança, agrupado por partida, precisa ficar inteiramente do lado favorável.

**Motivo:** o critério anterior comparava dois Brier e declarava vitória se um fosse menor. Com dados de chute reais, o modelo superou o baseline por `0,00006` e se autodeclarou qualificado — a **um portão** de armar alertas reais com dinheiro do usuário do outro lado. Um bootstrap agrupado por partida mostrou que a diferença era indistinguível de zero.

**Consequência:** é esperado e correto que o sistema permaneça em silêncio por longos períodos. Silêncio por ausência de evidência é o comportamento projetado, não um defeito a contornar.

---

## 29. Fontes e referências

### Google Cloud e Firebase

- [Google Cloud Free Program - recursos gratuitos](https://docs.cloud.google.com/free/docs/free-cloud-features?hl=pt-br)
- [Compute Engine Free Tier - e2-micro e disco](https://docs.cloud.google.com/free/docs/free-cloud-features?hl=pt-br)
- [Cloud Run pricing](https://cloud.google.com/run/pricing)
- [Preços de IPv4 externo](https://cloud.google.com/vpc/network-pricing?hl=pt-BR)
- [Firebase pricing](https://firebase.google.com/pricing?hl=pt-BR)
- [BigQuery pricing](https://cloud.google.com/bigquery/pricing)
- [Pub/Sub pricing](https://cloud.google.com/pubsub/pricing?hl=pt-br)
- [Cloud Scheduler pricing](https://cloud.google.com/scheduler/pricing?hl=pt-br)

### Créditos Google AI Pro

- [Google Developer Program - planos e preços](https://developers.google.com/program/plans-and-pricing?hl=pt-br)
- [Google One - benefícios do Google AI Pro](https://support.google.com/googleone/answer/14534406?hl=pt-br)
- [Google Developer Program - FAQ de benefícios](https://developers.google.com/profile/help/benefits?hl=pt-br)

### Dados esportivos

- [StatsBomb Open Data](https://github.com/statsbomb/open-data)
- [SportMonks Football API - planos e preços](https://www.sportmonks.com/football-api/plans-pricing/)

### Regulação e jogo responsável no Brasil

- [Secretaria de Prêmios e Apostas - apostas de quota fixa](https://www.gov.br/fazenda/pt-br/composicao/orgaos/secretaria-de-premios-e-apostas/apostas-de-quota-fixa)
- [Jogo Responsável - Ministério da Fazenda](https://www.gov.br/fazenda/pt-br/composicao/orgaos/secretaria-de-premios-e-apostas/jogo-responsavel/jogo-responsavel/)
- [Exigências de publicidade de apostas - julho de 2026](https://www.gov.br/fazenda/pt-br/assuntos/noticias/2026/julho/ministerio-da-fazenda-amplia-exigencias-de-publicidade-de-apostas-no-pais)

---

## 30. Conclusão

O MVP correto não é uma plataforma de apostas nem um oráculo de gols. É uma régua que responde, de maneira auditável:

> Este cenário que eu observo no futebol se repete com frequência suficiente para valer o preço que estão me oferecendo?

E que responde **"não"** quando é não.

A versão 1.0 apostava que prever gols melhor produziria valor. A medição refutou isso: o modelo empata com a taxa base do futebol, e nenhuma compra de dado ao alcance deste projeto cria a assimetria necessária para vencer um mercado eficiente.

O que sobreviveu à refutação foi o aparato construído para testá-la. Intervalos de confiança, agrupamento por partida, uma observação por oportunidade, contratos selados, e a disciplina de permanecer em silêncio sem evidência. Isso é raro no setor, e é a única peça deste projeto que não disputa espaço com quem tem dado melhor e décadas de vantagem.

Os princípios de arquitetura permanecem:

- um provider substituível;
- um log temporal confiável;
- replay antes de produção;
- modelos simples antes de modelos sofisticados;
- **medição antes de afirmação**;
- **intervalo antes de estimativa pontual**;
- custos baixos antes de escala;
- segurança e jogo responsável antes de monetização.

A próxima decisão de engenharia é tirar o backtester da linha de comando e colocá-lo numa tela onde o usuário monte o próprio cenário — a menor distância entre o que existe hoje e algo vendável.

> Uma ferramenta que só sabe encontrar oportunidade não é ferramenta, é vitrine. O valor do Golo está em ser confiável quando diz que não há nada ali.
