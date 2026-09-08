import React, { useRef } from 'react';
import Editor from '@monaco-editor/react';
import { Play, Code, RefreshCw } from 'lucide-react';

export default function MonacoEditorPane({
  code,
  onChange,
  onSubmit,
  isSubmitting,
  disabled,
  onResetCode,
}) {
  const editorRef = useRef(null);

  const handleEditorDidMount = (editor, monaco) => {
    editorRef.current = editor;

    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
      if (!disabled && !isSubmitting) {
        onSubmit();
      }
    });
  };

  return (
    <div className="h-full flex flex-col bg-[#0d1117] overflow-hidden">
      <div className="px-4 py-2.5 bg-[#161b22] border-b border-[#30363d] flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Code className="h-4 w-4 text-purple-400" />
          <span className="text-xs font-semibold text-[#f0f6fc]">Python 3</span>
          <span className="text-xs text-[#8b949e]">· solution.py</span>
        </div>

        <div className="flex items-center gap-2">
          {onResetCode && (
            <button
              onClick={onResetCode}
              disabled={disabled}
              title="Reset starter template"
              className="p-1.5 text-[#8b949e] hover:text-[#f0f6fc] hover:bg-[#21262d] rounded-lg transition-colors disabled:opacity-40 cursor-pointer"
            >
              <RefreshCw className="h-3.5 w-3.5" />
            </button>
          )}

          <button
            onClick={onSubmit}
            disabled={disabled || isSubmitting}
            className="flex items-center gap-2 bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 disabled:opacity-40 text-white text-xs font-bold px-4 py-1.5 rounded-lg shadow-md shadow-emerald-600/20 active:scale-95 transition-all cursor-pointer"
          >
            {isSubmitting ? (
              <>
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                <span>Running...</span>
              </>
            ) : (
              <>
                <Play className="h-3.5 w-3.5 fill-current" />
                <span>Submit (Ctrl+Enter)</span>
              </>
            )}
          </button>
        </div>
      </div>

      <div className="flex-1 w-full h-full relative">
        <Editor
          height="100%"
          defaultLanguage="python"
          language="python"
          theme="vs-dark"
          value={code}
          onChange={onChange}
          onMount={handleEditorDidMount}
          options={{
            readOnly: disabled,
            minimap: { enabled: false },
            fontSize: 14,
            fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace",
            lineNumbers: 'on',
            roundedSelection: false,
            scrollBeyondLastLine: false,
            automaticLayout: true,
            tabSize: 4,
            padding: { top: 12, bottom: 12 },
            cursorBlinking: 'smooth',
            suggestOnTriggerCharacters: true,
            folding: true,
          }}
        />
      </div>
    </div>
  );
}
