import { createContext, useContext, ReactNode } from 'react';

interface ThemeContextType {
  theme: 'light';
  toggleTheme: () => void;
  isDark: false;
}

const ThemeContext = createContext<ThemeContextType>({
  theme: 'light',
  toggleTheme: () => {},
  isDark: false,
});

export function useTheme() {
  return useContext(ThemeContext);
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  return (
    <ThemeContext.Provider value={{ theme: 'light', toggleTheme: () => {}, isDark: false }}>
      {children}
    </ThemeContext.Provider>
  );
}
