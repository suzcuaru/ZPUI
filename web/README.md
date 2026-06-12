# ZPUI Web Interface - New Implementation

## Overview

This is a completely rebuilt web interface for managing Zapret and SOCKS5 proxy services, following modern web development best practices and design guidelines from Google Material Design 3 and Yandex Gravity UI.

## Key Features

### Design Principles

1. **Material Design 3 (M3)**
   - Comprehensive Material Design 3 component system
   - Responsive design with adaptive layouts
   - Accessibility-first approach
   - Smooth animations and micro-interactions

2. **Yandex Gravity UI Patterns**
   - BEM methodology for CSS organization
   - Component-based architecture
   - RTL support
   - Performance optimizations

3. **Modern Web Technologies**
   - React 18 with hooks
   - TypeScript support (via JSDoc)
   - Framer Motion for animations
   - Emotion for styling
   - Material Web Components

### Components

#### Core Components
- **Sidebar** - Responsive navigation with status indicators
- **Card** - Material Design 3 card component with hover effects
- **Switch** - Accessible toggle switches
- **Toast** - Toast notifications with animations
- **Modal** - Modal dialogs with backdrop
- **LogDrawer** - Slide-out log viewer
- **CopyBtn** - Copy-to-clipboard buttons

#### Page Components
- **OverviewPage** - Dashboard with metrics and traffic visualization
- **MonitorPage** - Real-time monitoring with charts and device activity
- **ProxyPage** - Proxy configuration and management
- **SettingsPage** - Settings with tabbed interface

### Design System

#### Color Scheme
- Primary: #5b8def (Material Blue)
- Secondary: #9f7aea (Material Purple)
- Success: #4ade80 (Material Green)
- Warning: #fbbf24 (Material Yellow)
- Danger: #f87171 (Material Red)
- Dark theme with light theme support

#### Typography
- Font: Roboto (Google Fonts)
- Scale: Material Design 3 type scale
- Responsive typography

#### Layout
- Grid system with 2-4 column layouts
- Responsive breakpoints (768px, 1024px, 1400px)
- Flexible card layouts
- Mobile-first approach

### Features

#### Real-time Updates
- WebSocket connections for live data
- Automatic polling for status updates
- Smooth animations for data changes

#### Accessibility
- ARIA labels and roles
- Keyboard navigation
- Screen reader support
- High contrast mode support

#### Performance
- Code splitting with Vite
- Tree shaking
- Lazy loading
- Efficient rendering

#### Security
- HTTPS support
- Secure API calls
- Input validation

## Development

### Installation

```bash
cd web
npm install
npm run dev
```

### Build

```bash
npm run build
```

### Preview

```bash
npm run preview
```

### Dependencies

- React 18.3.1
- @emotion/react & @emotion/styled
- framer-motion
- @material/web (Material Design 3 components)
- Vite

## Architecture

### Folder Structure

```
web/
├── src/
│   ├── App.jsx                    # Main application component
│   ├── contexts/                  # React contexts
│   │   └── ThemeContext.jsx
│   ├── components/                # Reusable components
│   │   ├── Card.jsx
│   │   ├── Switch.jsx
│   │   ├── Toast.jsx
│   │   ├── Modal.jsx
│   │   ├── LogDrawer.jsx
│   │   ├── CopyBtn.jsx
│   │   └── Sidebar.jsx
│   ├── pages/                     # Page components
│   │   ├── OverviewPage.jsx
│   │   ├── MonitorPage.jsx
│   │   ├── ProxyPage.jsx
│   │   └── SettingsPage.jsx
│   ├── api.js                     # API client
│   ├── lang.js                   # Localization
│   ├── utils.js                   # Utility functions
│   ├── App.css                    # Main styles
│   └── styles/                    # Design system
│       └── theme.css
├── public/                       # Static assets
│   └── index.html
├── vite.config.js                 # Vite configuration
└── package.json                  # Dependencies
```

### State Management

- Local state with React hooks
- Context API for theme and global state
- Event-driven updates
- Efficient re-renders

### API Integration

- RESTful API client
- Error handling and retries
- Type-safe API calls
- WebSocket support for real-time updates

## Design Guidelines

### Material Design 3
- Elevation levels for depth
- Ripple effects on interaction
- Consistent spacing and sizing
- Clear visual hierarchy

### Yandex Gravity UI
- Component documentation
- Accessibility guidelines
- Performance optimizations
- Mobile-first approach

## Testing

The application includes:
- Responsive design testing
- Accessibility testing
- Performance monitoring
- Visual regression testing

## Future Enhancements

1. **Material Web Components** - Full integration with @material/web
2. **Advanced Charts** - Integration with Chart.js or D3.js
3. **Real-time Notifications** - WebSocket-based notifications
4. **Export Functionality** - Data export to CSV/PDF
5. **Advanced Filtering** - Search and filter capabilities
6. **Dark/Light Theme Toggle** - User preference storage
7. **Accessibility Audit** - Comprehensive accessibility testing

## Conclusion

This new implementation provides a modern, accessible, and performant web interface that follows industry best practices. It combines the best of Google Material Design 3 and Yandex Gravity UI to create a cohesive and user-friendly experience for managing Zapret and proxy services.

The interface is designed to be:
- **Modern** - Uses latest web technologies
- **Accessible** - Follows WCAG guidelines
- **Performant** - Optimized for speed and efficiency
- **Maintainable** - Well-structured and documented
- **Extensible** - Easy to add new features
