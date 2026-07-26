import React, { useState } from 'react';
import { Play, Pause, RotateCcw, Zap, FastForward, Film } from 'lucide-react';

interface ReplayControlProps {
  onControlAction: (action: string, speed?: string) => void;
}

export const ReplayControl: React.FC<ReplayControlProps> = ({ onControlAction }) => {
  const [isPlaying, setIsPlaying] = useState(true);
  const [speed, setSpeed] = useState<'1x' | '10x' | 'max'>('10x');

  const togglePlay = () => {
    const nextPlaying = !isPlaying;
    setIsPlaying(nextPlaying);
    onControlAction(nextPlaying ? 'play' : 'pause');
  };

  const handleSpeedChange = (newSpeed: '1x' | '10x' | 'max') => {
    setSpeed(newSpeed);
    onControlAction('speed', newSpeed);
  };

  const handleReset = () => {
    setIsPlaying(true);
    onControlAction('reset');
  };

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 sm:p-8 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-extrabold text-slate-50 flex items-center gap-2">
            <Film className="w-5 h-5 text-emerald-400" /> Replay Engine & Backtest Simulation
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Reproduza partidas históricas usando o mesmo pipeline determinístico do modo ao vivo.
          </p>
        </div>

        <span className="text-xs font-mono font-bold px-3 py-1 bg-emerald-500/10 text-emerald-400 rounded-full border border-emerald-500/20">
          REPLAY ACTIVE
        </span>
      </div>

      {/* Control Buttons Panel */}
      <div className="bg-slate-950 border border-slate-800 rounded-xl p-5 flex flex-col sm:flex-row items-center justify-between gap-4">
        {/* Play/Pause & Reset */}
        <div className="flex items-center gap-3">
          <button
            onClick={togglePlay}
            className="flex items-center gap-2 px-5 py-2.5 bg-emerald-500 hover:bg-emerald-400 text-slate-950 font-black rounded-xl text-sm transition-all shadow-lg shadow-emerald-500/20"
          >
            {isPlaying ? <Pause className="w-4 h-4 fill-slate-950" /> : <Play className="w-4 h-4 fill-slate-950" />}
            <span>{isPlaying ? 'Pausar Replay' : 'Iniciar Replay'}</span>
          </button>

          <button
            onClick={handleReset}
            className="flex items-center gap-2 px-4 py-2.5 bg-slate-900 hover:bg-slate-800 text-slate-300 rounded-xl text-xs font-bold border border-slate-800 transition-all"
          >
            <RotateCcw className="w-4 h-4" />
            <span>Reiniciar</span>
          </button>
        </div>

        {/* Speed Selector */}
        <div className="flex items-center gap-2">
          <span className="text-xs font-mono text-slate-400 mr-1 flex items-center gap-1">
            <Zap className="w-3.5 h-3.5 text-amber-400" /> Velocidade:
          </span>
          <button
            onClick={() => handleSpeedChange('1x')}
            className={`px-3 py-1.5 rounded-lg text-xs font-mono font-bold border transition-all ${
              speed === '1x'
                ? 'bg-slate-800 text-emerald-400 border-slate-700'
                : 'bg-slate-900 text-slate-400 border-slate-800 hover:text-slate-200'
            }`}
          >
            1x (Real)
          </button>

          <button
            onClick={() => handleSpeedChange('10x')}
            className={`px-3 py-1.5 rounded-lg text-xs font-mono font-bold border transition-all ${
              speed === '10x'
                ? 'bg-slate-800 text-emerald-400 border-slate-700'
                : 'bg-slate-900 text-slate-400 border-slate-800 hover:text-slate-200'
            }`}
          >
            10x (Inspeção)
          </button>

          <button
            onClick={() => handleSpeedChange('max')}
            className={`px-3 py-1.5 rounded-lg text-xs font-mono font-bold border transition-all ${
              speed === 'max'
                ? 'bg-slate-800 text-emerald-400 border-slate-700'
                : 'bg-slate-900 text-slate-400 border-slate-800 hover:text-slate-200'
            }`}
          >
            MAX (Backtest)
          </button>
        </div>
      </div>
    </div>
  );
};
