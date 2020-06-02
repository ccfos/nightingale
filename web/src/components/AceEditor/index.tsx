import React from 'react';
import AceEditor from 'react-ace';
import 'ace-builds/src-noconflict/mode-sh';
import 'ace-builds/src-noconflict/theme-monokai';

interface Props {
  style: any;
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
}

export default function Editor(props: Props) {
  return (
    <AceEditor
      placeholder={props.placeholder === undefined ? 'Placeholder Text' : props.placeholder}
      style={{ width: '100%', ...props.style }}
      mode="sh"
      theme="monokai"
      name="blah2"
      fontSize={14}
      showPrintMargin={false}
      showGutter
      highlightActiveLine
      setOptions={{
        enableBasicAutocompletion: true,
        enableLiveAutocompletion: true,
        enableSnippets: true,
        showLineNumbers: true,
        tabSize: 2,
      }}
      value={props.value}
      onChange={(newValue) => {
        props.onChange(newValue);
      }}
    />
  )
}
