import { createContext, useContext, useState } from 'react';

const ThemeContext = createContext();

export const useTheme = () => {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
};

export const ThemeProvider = ({ children }) => {
  const [isDarkMode, setIsDarkMode] = useState(true);

  const toggleTheme = () => {
    setIsDarkMode(prev => !prev);
  };

  const theme = {
    isDarkMode,
    toggleTheme,
    colors: isDarkMode
      ? {
          primary: '#5b8def',
          primaryContainer: 'rgba(91, 141, 239, 0.12)',
          secondary: '#9f7aea',
          background: '#111219',
          surface: '#1a1b27',
          surfaceVariant: '#242536',
          text: '#e4e4eb',
          textSecondary: '#9394a5',
          textTertiary: '#5f6073',
          border: '#2d2e42',
          error: '#f87171',
          warning: '#fbbf24',
          success: '#4ade80',
        }
      : {
          primary: '#5b8def',
          primaryContainer: 'rgba(91, 141, 239, 0.12)',
          secondary: '#9f7aea',
          background: '#ffffff',
          surface: '#f9fafb',
          surfaceVariant: '#f3f4f6',
          text: '#111827',
          textSecondary: '#4b5563',
          textTertiary: '#9ca3af',
          border: '#e5e7eb',
          error: '#ef4444',
          warning: '#f59e0b',
          success: '#10b981',
        },
  };

  return (
    <ThemeContext.Provider value={theme}>{children}</ThemeContext.Provider>
  );
};
