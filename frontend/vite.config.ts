import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          // Core vendor splits
          'vendor-react':   ['react','react-dom','react-router-dom'],
          'vendor-query':   ['@tanstack/react-query'],
          'vendor-charts':  ['recharts'],
          'vendor-ui':      ['lucide-react','clsx','react-hot-toast'],
          'vendor-utils':   ['axios','date-fns'],
          // Page group splits by capability domain
          'pages-observe':  [
            './src/pages/OverviewPage',
            './src/pages/ProblemsPage',
            './src/pages/EventsPage',
            './src/pages/SmartscapePage',
            './src/pages/MetricsPage',
            './src/pages/LogsPage',
            './src/pages/TracesPage',
            './src/pages/DashboardsPage',
          ],
          'pages-apm':      [
            './src/pages/APMPage',
            './src/pages/ServiceEndpointsPage',
            './src/pages/TiersNodesPage',
            './src/pages/BusinessTransactionsPage',
            './src/pages/ErrorTrackingPage',
            './src/pages/ErrorsInboxPage',
            './src/pages/ProfilingPage',
          ],
          'pages-infra':    [
            './src/pages/InfrastructurePage',
            './src/pages/KubernetesPage',
            './src/pages/DatabasePage',
            './src/pages/NetworkPage',
            './src/pages/ServerlessPage',
            './src/pages/MultiClusterPage',
          ],
          'pages-ai':       [
            './src/pages/AIAgentPage',
            './src/pages/LLMMonitoringPage',
            './src/pages/DeviationAnalysisPage',
            './src/pages/AnomalyDetectionPage',
            './src/pages/WatchdogPage',
            './src/pages/ForecastPage',
            './src/pages/AdaptiveTelemetryPage',
          ],
          'pages-devops':   [
            './src/pages/DORAPage',
            './src/pages/CIPipelinePage',
            './src/pages/LoadTestingPage',
            './src/pages/ChaosPage',
            './src/pages/ReleasesPage',
            './src/pages/ReleaseHealthPage',
            './src/pages/FeatureFlagsPage',
            './src/pages/DeploymentIntelligencePage',
          ],
          'pages-alerts':   [
            './src/pages/AlertRulesPage',
            './src/pages/IncidentsPage',
            './src/pages/SLOsPage',
            './src/pages/OnCallPage',
            './src/pages/AlertCorrelationPage',
            './src/pages/ChangeTrackingPage',
            './src/pages/WorkloadsPage',
            './src/pages/RunbooksPage',
          ],
          'pages-enterprise':[
            './src/pages/CompliancePage',
            './src/pages/SSOPage',
            './src/pages/AuthPage',
            './src/pages/SettingsPage',
            './src/pages/ServiceCatalogPage',
            './src/pages/TechMatrixPage',
            './src/pages/HubPage',
            './src/pages/AgentInstallPage',
            './src/pages/AgentFleetPage',
            './src/pages/IntegrationsPage',
          ],
          'pages-data':     [
            './src/pages/DataStreamsPage',
            './src/pages/BackgroundJobsPage',
            './src/pages/CronMonitorPage',
            './src/pages/PipelinePage',
            './src/pages/StatusPageBuilderPage',
            './src/pages/UptimePage',
          ],
          'pages-exp':      [
            './src/pages/RUMPage',
            './src/pages/MobilePage',
            './src/pages/SessionReplayPage',
            './src/pages/SyntheticPage',
          ],
        },
      },
    },
    chunkSizeWarningLimit: 600,
  },
})
