import React from 'react';

export default class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { error: null, info: null };
  }
  static getDerivedStateFromError(error) {
    return { error };
  }
  componentDidCatch(error, info) {
    this.setState({ info });
    console.error('[ErrorBoundary]', error, info);
  }
  render() {
    if (this.state.error) {
      const stack = (this.state.error && this.state.error.stack) || String(this.state.error);
      return (
        <div style={{
          padding: 24, height: '100vh', overflow: 'auto',
          background: '#15151c', color: '#ff7a7a',
          fontFamily: 'Consolas, monospace', fontSize: 13, whiteSpace: 'pre-wrap',
        }}>
          <h2 style={{ color: '#fff', marginBottom: 12 }}>React render error</h2>
          <div>{stack}</div>
          {this.state.info && this.state.info.componentStack && (
            <div style={{ opacity: 0.6, marginTop: 16 }}>{this.state.info.componentStack}</div>
          )}
          <button
            onClick={() => window.location.reload()}
            style={{ marginTop: 16, padding: '6px 12px', background: '#4f8ff7', color: '#fff', border: 'none', borderRadius: 6, cursor: 'pointer' }}
          >Reload</button>
        </div>
      );
    }
    return this.props.children;
  }
}
