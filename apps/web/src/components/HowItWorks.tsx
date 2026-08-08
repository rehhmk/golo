import React from 'react';
import { PageHeader } from './PageHeader';

const PIPELINE_STAGES = [
  { title: 'Provider', desc: 'Fonte de dados ao vivo. Retorna partidas e eventos brutos.' },
  { title: 'Normalizer', desc: 'Converte o formato do provider em um evento canônico único.' },
  { title: 'EventStore', desc: 'Grava cada evento em SQLite. Histórico auditável, nunca sobrescrito.' },
  { title: 'Reducer', desc: 'Função determinística. Mantém placar, cartões e janelas móveis.' },
  { title: 'Features', desc: 'Extrai o vetor no instante exato. Nunca usa dados futuros.' },
  { title: 'Predictor', desc: 'Aplica o modelo e a calibração, gerando probabilidades.' },
  { title: 'Publisher', desc: 'Publica estado e previsão para a interface.' },
];

// Every number rendered anywhere in the product should be findable here. The
// interface previously showed CA, ESC, VERM, pp and ECE with no expansion in
// any screen, and the maths page explained the integral while defining none of
// the words a reader actually meets.
const GLOSSARY: { term: string; meaning: string }[] = [
  {
    term: 'no alvo',
    meaning:
      'Chutes que foram na direção do gol nos últimos 10 minutos, somando os dois times. É o indicador de ataque que mais se relaciona com gol na nossa medição.',
  },
  {
    term: 'escanteios',
    meaning: 'Escanteios nos últimos 10 minutos, somando os dois times.',
  },
  {
    term: 'expulsão',
    meaning:
      'Há ao menos um jogador expulso em campo. Historicamente a taxa de gols sobe: 2,46 gols por 90 minutos sem expulsão, 2,59 com uma, 2,97 com duas.',
  },
  {
    term: 'xG',
    meaning:
      'Gols esperados — uma medida da qualidade das finalizações, não só da quantidade. Aparece zerado porque não está disponível no nosso plano de dados atual.',
  },
  {
    term: 'até o fim',
    meaning:
      'Probabilidade de sair pelo menos mais um gol entre agora e o apito final. Diminui conforme o tempo passa, porque sobra menos jogo.',
  },
  {
    term: 'dados: bons / medianos / fracos',
    meaning:
      'Qualidade do que estamos recebendo do provedor nesta partida — atraso, campos faltando, placar inconsistente. Não é a confiança do modelo na previsão.',
  },
  {
    term: 'acerto aqui',
    meaning:
      'Das nossas previsões de 10 minutos nesta partida que já se resolveram, quantas acertaram. Só aparece quando há previsões resolvidas suficientes.',
  },
  {
    term: 'Em alta',
    meaning: 'Filtro que mostra apenas partidas com 75% ou mais de chance de gol nos próximos 10 minutos.',
  },
  {
    term: 'taxa base',
    meaning:
      'Com que frequência o desfecho acontece sem nenhuma condição — em qualquer partida, a qualquer momento. Um cenário que não supera a taxa base não está informando nada.',
  },
  {
    term: 'odd mínima',
    meaning:
      'O preço abaixo do qual a aposta perde dinheiro no longo prazo. É 1 dividido pela taxa de acerto — e usamos o limite pessimista do intervalo, não a estimativa central.',
  },
  {
    term: 'intervalo de confiança',
    meaning:
      'A faixa onde a taxa real provavelmente está. Com 50 observações a 50% de acerto, a taxa real fica entre 36% e 63% — por isso nunca mostramos uma taxa sem a faixa dela.',
  },
  {
    term: 'pior sequência',
    meaning:
      'A maior sequência de perdas seguidas que esse cenário já teve no histórico. Observada, não estimada. É dela que sai o tamanho sugerido da entrada.',
  },
  {
    term: 'Brier',
    meaning:
      'Erro médio ao quadrado das nossas probabilidades. Zero seria perfeito. Só faz sentido comparado a outro modelo — por isso mostramos sempre ao lado da taxa constante.',
  },
  {
    term: 'log loss',
    meaning: 'Outra medida de erro, que pune com mais força uma previsão confiante e errada.',
  },
  {
    term: 'ECE',
    meaning:
      'Erro de calibração. Se dissermos 70% em cem situações, o esperado é acontecer em setenta delas. O ECE mede o quanto fugimos disso.',
  },
  {
    term: 'pp',
    meaning:
      'Pontos percentuais. A diferença entre 40% e 45% é de 5 pontos percentuais — não de 5%, que seria bem menos.',
  },
];

export const HowItWorks: React.FC = () => {
  return (
    <div className="max-w-3xl">
      <PageHeader
        label="Documentação"
        title="Como o Golo funciona"
        description="Da coleta de dados até a probabilidade na tela — sem caixa-preta."
      />

      <Prose>
        Em qualquer instante <Mono>t</Mono> de uma partida, o Golo usa apenas informação disponível{' '}
        <em className="text-slate-300 not-italic font-medium">até aquele instante</em> para estimar a probabilidade de
        sair pelo menos um gol dentro de um horizonte de tempo. A previsão nunca vê o futuro — nem ao vivo, nem em
        replay.
      </Prose>

      <Section n="01" title="O pipeline">
        <Prose>Cada evento passa pelas mesmas sete etapas, ao vivo ou em replay.</Prose>
        <ol className="mt-4 space-y-0 border-t border-white/[0.06]">
          {PIPELINE_STAGES.map((stage, i) => (
            <li
              key={stage.title}
              className="flex items-baseline gap-4 py-2.5 border-b border-white/[0.06]"
            >
              <span className="font-mono text-[10px] tabular-nums text-slate-700 w-5 shrink-0">
                {String(i + 1).padStart(2, '0')}
              </span>
              <span className="font-mono text-[12px] text-emerald-400/90 w-[92px] shrink-0">{stage.title}</span>
              <span className="text-[13px] text-slate-400">{stage.desc}</span>
            </li>
          ))}
        </ol>
      </Section>

      <Section n="02" title="O que a probabilidade significa">
        <Prose>
          Para um horizonte <Mono>h</Mono> — por exemplo 10 minutos — o Golo estima:
        </Prose>
        <Formula>P(gol entre t e t+h | tudo observado até t)</Formula>
      </Section>

      <Section n="03" title="Processo de intensidade">
        <Prose>
          A base é um processo de intensidade <Mono>λ</Mono>, a taxa instantânea de gol. A probabilidade de seguir sem
          gol decai exponencialmente com a intensidade acumulada:
        </Prose>
        <Formula>
          {'P(nenhum gol entre t e T) = exp( -∫ λ(u) du )\nP(pelo menos um gol)    = 1 - exp( -∫ λ(u) du )'}
        </Formula>
        <Prose>
          Na prática o tempo é discretizado em intervalos pequenos <Mono>Δt</Mono>:
        </Prose>
        <Formula>{'P(gol no horizonte) = 1 - exp( -Σ λ_k · Δt )'}</Formula>
      </Section>

      <Section n="04" title="De onde vem λ">
        <Prose>
          <Mono>λ</Mono> parte de um prior pré-jogo, baseado na força ofensiva e defensiva de cada time e no mando de
          campo:
        </Prose>
        <Formula>{'log(λ_pre) = intercepto_liga + ataque_mandante\n             + defesa_visitante + vantagem_mando'}</Formula>
        <Prose>
          Ao vivo, esse valor é ajustado por um multiplicador que reage ao que está acontecendo — minuto, placar,
          cartões, chutes, pressão recente:
        </Prose>
        <Formula>{'λ_t = λ_pre · exp( β · X_t )'}</Formula>
      </Section>

      <Section n="05" title="Calibração">
        <Prose>
          A probabilidade crua passa por um calibrador treinado separadamente (Platt scaling), para que &quot;70% de
          chance&quot; signifique de fato que cerca de 70% dessas previsões terminam em gol:
        </Prose>
        <Formula>{'P_calibrada = 1 / ( 1 + exp( A · logit(P_crua) + B ) )'}</Formula>
      </Section>

      <Section n="06" title="Horizontes">
        <Prose>Cinco probabilidades são calculadas em paralelo a cada atualização:</Prose>
        <ul className="mt-3 grid grid-cols-1 sm:grid-cols-2 gap-x-6 border-t border-white/[0.06]">
          {['Gol nos próximos 5 minutos', 'Gol nos próximos 10 minutos', 'Gol até o fim do jogo', 'Gol do mandante até o fim', 'Gol do visitante até o fim'].map(
            (h) => (
              <li key={h} className="text-[13px] text-slate-400 py-2 border-b border-white/[0.06]">
                {h}
              </li>
            )
          )}
        </ul>
      </Section>

      <Section n="07" title="Probabilidade não é confiança">
        <Prose>
          O Golo separa duas coisas que costumam ser confundidas: a <strong className="text-slate-200 font-medium">probabilidade
          do evento</strong> e a <strong className="text-slate-200 font-medium">confiança nos dados</strong> usados para
          calculá-la. Uma previsão de 70% com o feed atrasado não tem o mesmo peso de uma com dados em dia — por isso
          cada partida exibe sua banda de confiança ao lado do número.
        </Prose>
      </Section>

      <Section n="08" title="Glossário">
        <Prose>
          Toda palavra que aparece em algum número da tela está aqui. Se algo na
          interface não estiver explicado nesta lista, é falha nossa.
        </Prose>
        <dl className="mt-4 space-y-3">
          {GLOSSARY.map((entry) => (
            <div key={entry.term} className="grid grid-cols-1 sm:grid-cols-[150px_minmax(0,1fr)] gap-1 sm:gap-4">
              <dt className="font-mono text-[12px] text-slate-300">{entry.term}</dt>
              <dd className="text-[13px] leading-relaxed text-slate-400">{entry.meaning}</dd>
            </div>
          ))}
        </dl>
      </Section>

      <div className="mt-10 pt-5 border-t border-white/[0.08]">
        <div className="font-mono text-[10px] uppercase tracking-[0.14em] text-slate-600 mb-2">Limites do produto</div>
        <Prose>
          O Golo não usa LLM para calcular probabilidades, não executa apostas e não promete lucro. As probabilidades
          são estimativas sujeitas a erro, e a qualidade do dado ao vivo é uma limitação real do sistema.
        </Prose>
      </div>
    </div>
  );
};

const Section: React.FC<{ n: string; title: string; children: React.ReactNode }> = ({ n, title, children }) => (
  <section className="mt-10">
    <div className="flex items-baseline gap-3 mb-3">
      <span className="font-mono text-[10px] tabular-nums text-slate-700">{n}</span>
      <h2 className="text-[15px] font-medium text-slate-100">{title}</h2>
    </div>
    {children}
  </section>
);

const Prose: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <p className="text-[13.5px] leading-relaxed text-slate-400 mt-3 first:mt-0">{children}</p>
);

const Mono: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <code className="font-mono text-[12.5px] text-slate-200">{children}</code>
);

const Formula: React.FC<{ children: string }> = ({ children }) => (
  <pre className="mt-3 py-3 px-4 border-l-2 border-emerald-400/40 bg-white/[0.02] font-mono text-[12px] leading-relaxed text-slate-300 whitespace-pre-wrap overflow-x-auto">
    {children}
  </pre>
);
