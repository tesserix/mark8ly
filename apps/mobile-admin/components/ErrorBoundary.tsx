import { Component, type ReactNode } from 'react';
import { Pressable, View } from 'react-native';
import { Text } from './ui/Text';

interface Props {
  children: ReactNode;
}
interface State {
  error: Error | null;
}

// Top-level boundary so a render error surfaces a readable screen instead of
// a white crash. It logs today and offers in-session recovery via "Try again";
// crash reporting is wired through the single `reportError` seam below.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error) {
    this.reportError(error);
  }

  // Single telemetry seam. Today it only logs to the console; when a crash
  // reporter is adopted (e.g. Sentry / Crashlytics) wire the capture call HERE
  // and nowhere else. No such SDK is installed yet, so we deliberately do not
  // import one.
  private reportError(error: Error) {
    console.error('[mobile-admin] render error', error);
  }

  // Clear the caught error so the subtree re-mounts. Lets the user retry in
  // session instead of being forced to restart the whole app.
  private handleRetry = () => {
    this.setState({ error: null });
  };

  render() {
    if (this.state.error) {
      return (
        <View className="flex-1 items-center justify-center bg-paper px-6">
          <Text preset="h2" className="text-center">
            Something went wrong
          </Text>
          <Text preset="body" className="mt-2 text-center text-ink-muted">
            An unexpected error interrupted this screen.
          </Text>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel="Try again"
            onPress={this.handleRetry}
            className="mt-6 min-h-touch items-center justify-center rounded bg-ink px-6 active:opacity-90"
          >
            <Text preset="bodyEmphasis" className="text-paper">
              Try again
            </Text>
          </Pressable>
        </View>
      );
    }
    return this.props.children;
  }
}
