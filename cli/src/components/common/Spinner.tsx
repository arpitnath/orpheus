//@SPINNER_COMPONENT
import React from 'react';
import { Text } from 'ink';
import InkSpinner from 'ink-spinner';

interface SpinnerProps {
  label?: string;
  color?: string;
}

export const Spinner: React.FC<SpinnerProps> = ({ label, color = 'cyan' }) => {
  return (
    <Text>
      <Text color={color}>
        <InkSpinner type="dots" />
      </Text>
      {label && <Text> {label}</Text>}
    </Text>
  );
};
