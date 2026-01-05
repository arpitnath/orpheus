import React from 'react';
import { Box, Text } from 'ink';

interface TabBarProps {
  tabs: string[];
  activeIndex: number;
}

export const TabBar: React.FC<TabBarProps> = ({ tabs, activeIndex }) => (
  <Box>
    {tabs.map((tab, index) => {
      const isActive = index === activeIndex;
      return (
        <React.Fragment key={tab}>
          {isActive ? (
            <Text bold> {tab} </Text>
          ) : (
            <Text dimColor> {tab} </Text>
          )}
          <Text>  </Text>
        </React.Fragment>
      );
    })}
    <Text dimColor>(tab to cycle)</Text>
  </Box>
);
