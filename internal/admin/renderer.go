package admin

import (
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/rampantspark/gospidertrap/internal/stats"
)

// Renderer handles HTML generation for the admin UI.
type Renderer struct {
	adminPath string
}

// NewRenderer creates a new renderer.
//
// Parameters:
//   - adminPath: the admin endpoint path
//
// Returns a new Renderer instance.
func NewRenderer(adminPath string) *Renderer {
	return &Renderer{
		adminPath: adminPath,
	}
}

// RenderAdminUI generates the complete admin UI HTML.
//
// Parameters:
//   - chartData: data for charts (top IPs and user agents)
//   - uptime: server uptime duration
//   - totalRequests: total number of requests
//   - uniqueIPs: number of unique IP addresses
//   - uniqueUAs: number of unique user agents
//   - recentRequests: slice of recent request entries
//   - maxDisplay: maximum number of recent requests to display
//
// Returns the complete HTML as a string.
func (r *Renderer) RenderAdminUI(
	chartData stats.ChartData,
	uptime time.Duration,
	totalRequests, uniqueIPs, uniqueUAs int,
	recentRequests []stats.RequestInfo,
	maxDisplay int,
) string {
	var sb strings.Builder

	r.writeHTMLHeader(&sb)
	r.writeStatsBox(&sb, uptime, totalRequests, uniqueIPs, uniqueUAs)
	r.writeTopIPsSection(&sb, chartData)
	r.writeTopUAsSection(&sb, chartData)
	r.writeRecentRequestsSection(&sb, recentRequests, maxDisplay)
	r.writeChartScript(&sb)
	sb.WriteString("</body>\n</html>")

	return sb.String()
}

// writeHTMLHeader writes the HTML header, styles, and opening body tag.
func (r *Renderer) writeHTMLHeader(sb *strings.Builder) {
	sb.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	sb.WriteString("<title>gospidertrap - dashboard</title>\n")
	sb.WriteString("<style>\n")
	sb.WriteString("body { font-family: monospace; margin: 20px; background: #f5f5f5; }\n")
	sb.WriteString("h1 { color: #333; }\n")
	sb.WriteString(".stat-box { background: white; padding: 15px; margin: 10px 0; border-radius: 5px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }\n")
	sb.WriteString("table { width: 100%; border-collapse: collapse; margin-top: 10px; }\n")
	sb.WriteString("th, td { padding: 8px; text-align: left; border-bottom: 1px solid #ddd; }\n")
	sb.WriteString("th { background-color: #4CAF50; color: white; }\n")
	sb.WriteString("tr:hover { background-color: #f5f5f5; }\n")
	sb.WriteString(".ip { font-family: monospace; }\n")
	sb.WriteString(".chart-container { position: relative; height: 300px; margin: 20px 0; }\n")
	sb.WriteString(".charts-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin: 20px 0; }\n")
	sb.WriteString("@media (max-width: 768px) { .charts-grid { grid-template-columns: 1fr; } }\n")
	sb.WriteString(".chart-table-row { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin: 20px 0; }\n")
	sb.WriteString("@media (max-width: 768px) { .chart-table-row { grid-template-columns: 1fr; } }\n")
	sb.WriteString("</style>\n")
	sb.WriteString("<script src=\"https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js\"></script>\n")
	sb.WriteString("</head>\n<body>\n")
	sb.WriteString("<h1>gospidertrap</h1>\n")
}

// writeStatsBox writes the overall server statistics box.
func (r *Renderer) writeStatsBox(sb *strings.Builder, uptime time.Duration, totalRequests, uniqueIPs, uniqueUAs int) {
	sb.WriteString("<div class=\"stat-box\">\n")
	sb.WriteString("<h2>Server Statistics</h2>\n")
	sb.WriteString("<p><strong>Uptime:</strong> " + html.EscapeString(uptime.String()) + "</p>\n")
	sb.WriteString("<p><strong>Total Requests:</strong> " + strconv.Itoa(totalRequests) + "</p>\n")
	sb.WriteString("<p><strong>Unique IPs:</strong> " + strconv.Itoa(uniqueIPs) + "</p>\n")
	sb.WriteString("<p><strong>Unique User Agents:</strong> " + strconv.Itoa(uniqueUAs) + "</p>\n")
	sb.WriteString("</div>\n")
}

// writeTopIPsSection writes the top IP addresses chart and table section.
func (r *Renderer) writeTopIPsSection(sb *strings.Builder, chartData stats.ChartData) {
	sb.WriteString("<div class=\"chart-table-row\">\n")
	// Top IP Addresses Chart
	sb.WriteString("<div class=\"stat-box\">\n")
	sb.WriteString("<h2>Top IP Addresses</h2>\n")
	sb.WriteString("<div class=\"chart-container\"><canvas id=\"ipChart\"></canvas></div>\n")
	sb.WriteString("</div>\n")

	// Top IP Addresses Table
	if len(chartData.TopIPs.Labels) > 0 {
		sb.WriteString("<div class=\"stat-box\">\n")
		sb.WriteString("<h2>Top IP Addresses</h2>\n")
		sb.WriteString("<table>\n")
		sb.WriteString("<tr><th>IP Address</th><th>Request Count</th></tr>\n")

		for i := 0; i < len(chartData.TopIPs.Labels); i++ {
			sb.WriteString("<tr><td class=\"ip\">")
			sb.WriteString(html.EscapeString(chartData.TopIPs.Labels[i]))
			sb.WriteString("</td><td>")
			sb.WriteString(strconv.Itoa(chartData.TopIPs.Data[i]))
			sb.WriteString("</td></tr>\n")
		}
		sb.WriteString("</table>\n")
		sb.WriteString("</div>\n")
	}
	sb.WriteString("</div>\n")
}

// writeTopUAsSection writes the top user agents chart and table section.
func (r *Renderer) writeTopUAsSection(sb *strings.Builder, chartData stats.ChartData) {
	sb.WriteString("<div class=\"chart-table-row\">\n")
	// Top User Agents Chart
	sb.WriteString("<div class=\"stat-box\">\n")
	sb.WriteString("<h2>Top User Agents</h2>\n")
	sb.WriteString("<div class=\"chart-container\"><canvas id=\"uaChart\"></canvas></div>\n")
	sb.WriteString("</div>\n")

	// Top User Agents Table
	if len(chartData.TopUserAgents.Labels) > 0 {
		sb.WriteString("<div class=\"stat-box\">\n")
		sb.WriteString("<h2>Top User Agents</h2>\n")
		sb.WriteString("<table>\n")
		sb.WriteString("<tr><th>User Agent</th><th>Request Count</th></tr>\n")

		for i := 0; i < len(chartData.TopUserAgents.Labels); i++ {
			sb.WriteString("<tr><td>")
			sb.WriteString(html.EscapeString(chartData.TopUserAgents.Labels[i]))
			sb.WriteString("</td><td>")
			sb.WriteString(strconv.Itoa(chartData.TopUserAgents.Data[i]))
			sb.WriteString("</td></tr>\n")
		}
		sb.WriteString("</table>\n")
		sb.WriteString("</div>\n")
	}
	sb.WriteString("</div>\n")
}

// writeRecentRequestsSection writes the recent requests table section.
func (r *Renderer) writeRecentRequestsSection(sb *strings.Builder, recentRequests []stats.RequestInfo, maxDisplay int) {
	sb.WriteString("<div class=\"stat-box\">\n")
	sb.WriteString("<h2>Recent Requests</h2>\n")
	if len(recentRequests) > 0 {
		sb.WriteString("<table>\n")
		sb.WriteString("<tr><th>Timestamp</th><th>IP Address</th><th>Path</th><th>User Agent</th></tr>\n")

		// Display requests in order (Manager guarantees "most recent first")
		displayCount := len(recentRequests)
		if maxDisplay > 0 && maxDisplay < displayCount {
			displayCount = maxDisplay
		}

		for i := 0; i < displayCount; i++ {
			req := recentRequests[i]
			sb.WriteString("<tr><td>")
			sb.WriteString(html.EscapeString(req.Timestamp.Format("2006-01-02 15:04:05")))
			sb.WriteString("</td><td class=\"ip\">")
			sb.WriteString(html.EscapeString(req.IP))
			sb.WriteString("</td><td>")
			sb.WriteString(html.EscapeString(req.Path))
			sb.WriteString("</td><td>")
			sb.WriteString(html.EscapeString(req.UserAgent))
			sb.WriteString("</td></tr>\n")
		}
		sb.WriteString("</table>\n")
	} else {
		sb.WriteString("<p>No recent requests yet.</p>\n")
	}
	sb.WriteString("</div>\n")
}

// writeChartScript writes the JavaScript code for loading and rendering charts.
func (r *Renderer) writeChartScript(sb *strings.Builder) {
	sb.WriteString("<script>\n")
	sb.WriteString("const baseDataUrl = '")
	sb.WriteString(html.EscapeString(r.adminPath))
	sb.WriteString("/data';\n")
	sb.WriteString("let ipChart = null;\n")
	sb.WriteString("let uaChart = null;\n")
	sb.WriteString("\n")
	sb.WriteString("async function loadCharts() {\n")
	sb.WriteString("  try {\n")
	sb.WriteString("    const response = await fetch(baseDataUrl);\n")
	sb.WriteString("    if (!response.ok) throw new Error('Failed to load chart data');\n")
	sb.WriteString("    const data = await response.json();\n")
	sb.WriteString("\n")
	sb.WriteString("    // Top IPs Donut Chart\n")
	sb.WriteString("    if (!ipChart) {\n")
	sb.WriteString("      ipChart = new Chart(document.getElementById('ipChart'), {\n")
	sb.WriteString("      type: 'doughnut',\n")
	sb.WriteString("      data: {\n")
	sb.WriteString("        labels: data.topIPs.labels,\n")
	sb.WriteString("        datasets: [{\n")
	sb.WriteString("          data: data.topIPs.data,\n")
	sb.WriteString("          backgroundColor: [\n")
	sb.WriteString("            'rgba(255, 99, 132, 0.8)',\n")
	sb.WriteString("            'rgba(54, 162, 235, 0.8)',\n")
	sb.WriteString("            'rgba(255, 206, 86, 0.8)',\n")
	sb.WriteString("            'rgba(75, 192, 192, 0.8)',\n")
	sb.WriteString("            'rgba(153, 102, 255, 0.8)',\n")
	sb.WriteString("            'rgba(255, 159, 64, 0.8)',\n")
	sb.WriteString("            'rgba(199, 199, 199, 0.8)',\n")
	sb.WriteString("            'rgba(83, 102, 255, 0.8)',\n")
	sb.WriteString("          ]\n")
	sb.WriteString("        }]\n")
	sb.WriteString("      },\n")
	sb.WriteString("      options: {\n")
	sb.WriteString("        responsive: true,\n")
	sb.WriteString("        maintainAspectRatio: false,\n")
	sb.WriteString("        plugins: {\n")
	sb.WriteString("          legend: { position: 'bottom' }\n")
	sb.WriteString("        }\n")
	sb.WriteString("      }\n")
	sb.WriteString("    });\n")
	sb.WriteString("    }\n")
	sb.WriteString("\n")
	sb.WriteString("    // Top User Agents Donut Chart\n")
	sb.WriteString("    if (!uaChart) {\n")
	sb.WriteString("      uaChart = new Chart(document.getElementById('uaChart'), {\n")
	sb.WriteString("      type: 'doughnut',\n")
	sb.WriteString("      data: {\n")
	sb.WriteString("        labels: data.topUserAgents.labels,\n")
	sb.WriteString("        datasets: [{\n")
	sb.WriteString("          data: data.topUserAgents.data,\n")
	sb.WriteString("          backgroundColor: [\n")
	sb.WriteString("            'rgba(255, 99, 132, 0.8)',\n")
	sb.WriteString("            'rgba(54, 162, 235, 0.8)',\n")
	sb.WriteString("            'rgba(255, 206, 86, 0.8)',\n")
	sb.WriteString("            'rgba(75, 192, 192, 0.8)',\n")
	sb.WriteString("            'rgba(153, 102, 255, 0.8)',\n")
	sb.WriteString("            'rgba(255, 159, 64, 0.8)',\n")
	sb.WriteString("            'rgba(199, 199, 199, 0.8)',\n")
	sb.WriteString("            'rgba(83, 102, 255, 0.8)'\n")
	sb.WriteString("          ]\n")
	sb.WriteString("        }]\n")
	sb.WriteString("      },\n")
	sb.WriteString("      options: {\n")
	sb.WriteString("        responsive: true,\n")
	sb.WriteString("        maintainAspectRatio: false,\n")
	sb.WriteString("        plugins: {\n")
	sb.WriteString("          legend: { position: 'bottom' }\n")
	sb.WriteString("        }\n")
	sb.WriteString("      }\n")
	sb.WriteString("    });\n")
	sb.WriteString("    }\n")
	sb.WriteString("  } catch (error) {\n")
	sb.WriteString("    console.error('Error loading charts:', error);\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString("\n")
	sb.WriteString("loadCharts();\n")
	sb.WriteString("</script>\n")
}
