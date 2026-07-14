import { Component, type ReactNode } from 'react';
import { View } from 'react-native';
import { Text } from './ui/Text';

interface Props {
  children: ReactNode;
}
interface State {
  error: Error | null;
}

// Top-level boundary so a render error surfaces a readable screen instead of
// a white crash. Logged to console for now; wired to telemetry in Phase 4.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error) {
    console.error('[mobile-admin] render error', error);
  }

  render() {
    if (this.state.error) {
      return (
        <View className="flex-1 items-center justify-center bg-paper px-6">
          <Text preset="h2" className="text-center">
            Something went wrong
          </Text>
          <Text preset="body" className="mt-2 text-center text-ink-muted">
            Restart the app to continue.
          </Text>
        </View>
      );
    }
    return this.props.children;
  }
}
