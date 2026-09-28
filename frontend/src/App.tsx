import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'react-hot-toast'
import { useAuth } from '@/store/auth'
import { Layout } from '@/components/shared/Layout'

// Core
import LoginPage from '@/pages/LoginPage'
import OverviewPage from '@/pages/OverviewPage'

// Observe & Explore
import ProblemsPage from '@/pages/ProblemsPage'
import EventsPage from '@/pages/EventsPage'
import SmartscapePage from '@/pages/SmartscapePage'
import TopologyPage from '@/pages/TopologyPage'
import LogsPage from '@/pages/LogsPage'
import MetricsPage from '@/pages/MetricsPage'
import TracesPage from '@/pages/TracesPage'
import NotebooksPage from '@/pages/NotebooksPage'
import DashboardsPage from '@/pages/DashboardsPage'

// Application
import APMPage from '@/pages/APMPage'
import BusinessTransactionsPage from '@/pages/BusinessTransactionsPage'
import ServiceEndpointsPage from '@/pages/ServiceEndpointsPage'
import TiersNodesPage from '@/pages/TiersNodesPage'
import ErrorTrackingPage from '@/pages/ErrorTrackingPage'
import ErrorsInboxPage from '@/pages/ErrorsInboxPage'
import ProfilingPage from '@/pages/ProfilingPage'
import ReleaseHealthPage from '@/pages/ReleaseHealthPage'

// Digital Experience
import RUMPage from '@/pages/RUMPage'
import MobilePage from '@/pages/MobilePage'
import SessionReplayPage from '@/pages/SessionReplayPage'
import SyntheticPage from '@/pages/SyntheticPage'
import UptimePage from '@/pages/UptimePage'

// Infrastructure
import InfrastructurePage from '@/pages/InfrastructurePage'
import KubernetesPage from '@/pages/KubernetesPage'
import DatabasePage from '@/pages/DatabasePage'
import NetworkPage from '@/pages/NetworkPage'
import ServerlessPage from '@/pages/ServerlessPage'

// Data & Pipelines
import DataStreamsPage from '@/pages/DataStreamsPage'
import BackgroundJobsPage from '@/pages/BackgroundJobsPage'
import CronMonitorPage from '@/pages/CronMonitorPage'
import PipelinePage from '@/pages/PipelinePage'

// AI & Intelligence
import AIAgentPage from '@/pages/AIAgentPage'
import DeviationAnalysisPage from '@/pages/DeviationAnalysisPage'
import LLMMonitoringPage from '@/pages/LLMMonitoringPage'
import WatchdogPage from '@/pages/WatchdogPage'
import AnomalyDetectionPage from '@/pages/AnomalyDetectionPage'
import ForecastPage from '@/pages/ForecastPage'
import AdaptiveTelemetryPage from '@/pages/AdaptiveTelemetryPage'

// Alerts & Incidents
import AlertRulesPage from '@/pages/AlertRulesPage'
import IncidentsPage from '@/pages/IncidentsPage'
import SLOsPage from '@/pages/SLOsPage'
import ChangeTrackingPage from '@/pages/ChangeTrackingPage'
import WorkloadsPage from '@/pages/WorkloadsPage'
import OnCallPage from '@/pages/OnCallPage'
import RunbooksPage from '@/pages/RunbooksPage'
import StatusPageBuilderPage from '@/pages/StatusPageBuilderPage'
import PostmortemPage from '@/pages/PostmortemPage'

// Security
import SecurityPage from '@/pages/SecurityPage'
import AuthPage from '@/pages/AuthPage'
import CompliancePage from '@/pages/CompliancePage'

// DevOps & Delivery
import DORAPage from '@/pages/DORAPage'
import CIPipelinePage from '@/pages/CIPipelinePage'
import LoadTestingPage from '@/pages/LoadTestingPage'
import ChaosPage from '@/pages/ChaosPage'
import FeatureFlagsPage from '@/pages/FeatureFlagsPage'
import ReleasesPage from '@/pages/ReleasesPage'
import DeploymentIntelligencePage from '@/pages/DeploymentIntelligencePage'
import KPIPage from '@/pages/KPIPage'

// Enterprise
import ServiceCatalogPage from '@/pages/ServiceCatalogPage'
import TechMatrixPage from '@/pages/TechMatrixPage'
import APICatalogPage from '@/pages/APICatalogPage'
import SSOPage from '@/pages/SSOPage'
import SettingsPage from '@/pages/SettingsPage'

// Deploy & Connect
import HubPage from '@/pages/HubPage'
import AgentInstallPage from '@/pages/AgentInstallPage'
import AgentFleetPage from '@/pages/AgentFleetPage'
import IntegrationsPage from '@/pages/IntegrationsPage'

// Misc
import MultiClusterPage from '@/pages/MultiClusterPage'
import SystemHealthPage from '@/pages/SystemHealthPage'
import CostPage from '@/pages/CostPage'
import FinOpsPage from '@/pages/FinOpsPage'
import AlertCorrelationPage from '@/pages/AlertCorrelationPage'
import EventExplorerPage from '@/pages/EventExplorerPage'

// Detail pages (drill-down from list pages)
import ServiceDetailPage from "@/pages/ServiceDetailPage"
import ProblemDetailPage from "@/pages/ProblemDetailPage"
import TraceDetailPage from "@/pages/TraceDetailPage"
import IncidentDetailPage from "@/pages/IncidentDetailPage"
import HostDetailPage from "@/pages/HostDetailPage"
import DatabaseDetailPage from "@/pages/DatabaseDetailPage"
import PodDetailPage from "@/pages/PodDetailPage"
const qc = new QueryClient({ defaultOptions: { queries: { retry:1, staleTime:30_000, refetchOnWindowFocus:false } } })

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { token, user, loadMe, isLoading } = useAuth()
  const location = useLocation()
  useEffect(() => { if (token && !user && !isLoading) loadMe() }, [token, user, isLoading, loadMe])
  if (!token) return <Navigate to="/login" state={{ from: location }} replace />
  if (token && !user && isLoading) return (
    <div style={{ minHeight:'100vh', background:'#070f1e', display:'flex', alignItems:'center', justifyContent:'center' }}>
      <svg style={{ animation:'spin 1s linear infinite', width:32, height:32, color:'#6c72ff' }} viewBox="0 0 24 24">
        <circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" strokeWidth="3" strokeDasharray="31.4" strokeDashoffset="10"/>
      </svg>
    </div>
  )
  return <>{children}</>
}

export default function App() {
  return (
    <QueryClientProvider client={qc}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route element={<RequireAuth><Layout /></RequireAuth>}>
            {/* Observe & Explore */}
            <Route index element={<OverviewPage />} />
            <Route path="problems" element={<ProblemsPage />} />
            <Route path="problems/:id" element={<ProblemDetailPage />} />
            <Route path="events" element={<EventsPage />} />
            <Route path="smartscape" element={<SmartscapePage />} />
            <Route path="topology" element={<TopologyPage />} />
            <Route path="logs" element={<LogsPage />} />
            <Route path="metrics" element={<MetricsPage />} />
            <Route path="traces" element={<TracesPage />} />
            <Route path="traces/:traceId" element={<TraceDetailPage />} />
            <Route path="notebooks" element={<NotebooksPage />} />
            <Route path="dashboards" element={<DashboardsPage />} />
            {/* Application */}
            <Route path="apm" element={<APMPage />} />
            <Route path="services/:id" element={<ServiceDetailPage />} />
            <Route path="business-transactions" element={<BusinessTransactionsPage />} />
            <Route path="service-endpoints" element={<ServiceEndpointsPage />} />
            <Route path="tiers-nodes" element={<TiersNodesPage />} />
            <Route path="errors" element={<ErrorTrackingPage />} />
            <Route path="errors-inbox" element={<ErrorsInboxPage />} />
            <Route path="profiling" element={<ProfilingPage />} />
            <Route path="release-health" element={<ReleaseHealthPage />} />
            {/* Digital Experience */}
            <Route path="rum" element={<RUMPage />} />
            <Route path="mobile" element={<MobilePage />} />
            <Route path="session-replay" element={<SessionReplayPage />} />
            <Route path="synthetic" element={<SyntheticPage />} />
            <Route path="uptime" element={<UptimePage />} />
            {/* Infrastructure */}
            <Route path="infrastructure" element={<InfrastructurePage />} />
            <Route path="infrastructure/:hostId" element={<HostDetailPage />} />
            <Route path="kubernetes" element={<KubernetesPage />} />
            <Route path="kubernetes/pods/:podName" element={<PodDetailPage />} />
            <Route path="database" element={<DatabasePage />} />
            <Route path="database/:id" element={<DatabaseDetailPage />} />
            <Route path="network" element={<NetworkPage />} />
            <Route path="serverless" element={<ServerlessPage />} />
            {/* Data & Pipelines */}
            <Route path="data-streams" element={<DataStreamsPage />} />
            <Route path="background-jobs" element={<BackgroundJobsPage />} />
            <Route path="cron-monitor" element={<CronMonitorPage />} />
            <Route path="pipeline" element={<PipelinePage />} />
            {/* AI & Intelligence */}
            <Route path="ai-agent" element={<AIAgentPage />} />
            <Route path="deviation-analysis" element={<DeviationAnalysisPage />} />
            <Route path="llm-monitor" element={<LLMMonitoringPage />} />
            <Route path="watchdog" element={<WatchdogPage />} />
            <Route path="anomaly" element={<AnomalyDetectionPage />} />
            <Route path="forecast" element={<ForecastPage />} />
            <Route path="adaptive-telemetry" element={<AdaptiveTelemetryPage />} />
            {/* Alerts & Incidents */}
            <Route path="alerts" element={<AlertRulesPage />} />
            <Route path="incidents" element={<IncidentsPage />} />
            <Route path="incidents/:id" element={<IncidentDetailPage />} />
            <Route path="slos" element={<SLOsPage />} />
            <Route path="change-tracking" element={<ChangeTrackingPage />} />
            <Route path="workloads" element={<WorkloadsPage />} />
            <Route path="oncall" element={<OnCallPage />} />
            <Route path="runbooks" element={<RunbooksPage />} />
            <Route path="status-page" element={<StatusPageBuilderPage />} />
            <Route path="postmortems" element={<PostmortemPage />} />
            {/* Security */}
            <Route path="security" element={<SecurityPage />} />
            <Route path="auth" element={<AuthPage />} />
            <Route path="compliance" element={<CompliancePage />} />
            {/* DevOps */}
            <Route path="dora" element={<DORAPage />} />
            <Route path="ci-pipeline" element={<CIPipelinePage />} />
            <Route path="load-testing" element={<LoadTestingPage />} />
            <Route path="chaos" element={<ChaosPage />} />
            <Route path="feature-flags" element={<FeatureFlagsPage />} />
            <Route path="releases" element={<ReleasesPage />} />
            <Route path="deployment-intelligence" element={<DeploymentIntelligencePage />} />
            <Route path="kpis" element={<KPIPage />} />
            {/* Enterprise */}
            <Route path="catalog" element={<ServiceCatalogPage />} />
            <Route path="tech-matrix" element={<TechMatrixPage />} />
            <Route path="api-catalog" element={<APICatalogPage />} />
            <Route path="sso" element={<SSOPage />} />
            <Route path="settings" element={<SettingsPage />} />
            {/* Deploy */}
            <Route path="hub" element={<HubPage />} />
            <Route path="agent-install" element={<AgentInstallPage />} />
            <Route path="agent-fleet" element={<AgentFleetPage />} />
            <Route path="integrations" element={<IntegrationsPage />} />
            {/* Misc */}
            <Route path="multi-cluster" element={<MultiClusterPage />} />
            <Route path="system-health" element={<SystemHealthPage />} />
            <Route path="cost" element={<CostPage />} />
            <Route path="finops" element={<FinOpsPage />} />
            <Route path="correlation" element={<AlertCorrelationPage />} />
            <Route path="event-explorer" element={<EventExplorerPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
      <Toaster position="bottom-right" toastOptions={{
        style: { background:'#0b1628', color:'#c8d8ef', border:'1px solid #1a2d4a', fontSize:13 },
        success: { iconTheme: { primary:'#0fcf8a', secondary:'#0b1628' } },
        error:   { iconTheme: { primary:'#ff4d6a', secondary:'#0b1628' } },
      }}/>
    </QueryClientProvider>
  )
}
