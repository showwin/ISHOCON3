interface TeamScore {
    team: string;
    score: number;
    timestamp: string;
    language?: string;
}

declare const confetti: any;

const baseUrl = "<<API_GATEWAY_DOMAIN_URL>>";
const apiUrl = `${baseUrl}teams`;
const closedAtApiUrl = `${baseUrl}scoreboard/closed_at`;

const countdownKey = "countdown";
const partyKey = "party"
let countdownTimerId: number | null = null;

// Function to fetch the latest data
async function fetchData() {
    try {
        const response = await fetch(apiUrl);

        // 403 means the scoreboard API is closed, 1hr before the end of the contest
        if (response.status === 403) {
            const closedResponse = await fetch(closedAtApiUrl);
            const closedJson = await closedResponse.json();

            const ts = closedJson.timestamp
            const closedAtMs = Date.parse(ts + "Z"); // Append 'Z' to indicate UTC
            localStorage.setItem(countdownKey, String(closedAtMs));

            showFinalCountdown();
            return [];
        }

        // Reset countdown state if it existed
        if (localStorage.getItem(countdownKey)) {
            localStorage.removeItem(countdownKey);
        }

        // Clear countdown timer if running
        if (countdownTimerId !== null) {
            clearInterval(countdownTimerId);
            countdownTimerId = null;
        }

        // Call party when scoreboard re-open
        if (localStorage.getItem(partyKey)) {
            party();
            localStorage.removeItem(partyKey);
        }

        const data = await response.json();
        return data;
    } catch (error) {
        console.error("Error fetching data:", error);
        return [];
    }
}

// Define color scale globally to use across both graphs
const colorScale = d3.scaleOrdinal(d3.schemeTableau10);

function renderTimeline(data: TeamScore[]) {
    const margin = { top: 40, right: 30, bottom: 50, left: 80 };  // Adjust left margin for larger Y-axis values
    const width = document.body.clientWidth * 0.8 - margin.left - margin.right;
    const height = 400 - margin.top - margin.bottom;

    // Create a title for the graph
    d3.select("#timeline-chart")
        .append("h2")
        .style("text-align", "center")
        .text("Timeline");

    const svg = d3.select("#timeline-chart")
        .append("svg")
        .attr("width", width + margin.left + margin.right)
        .attr("height", height + margin.top + margin.bottom)
        .append("g")
        .attr("transform", `translate(${margin.left},${margin.top})`);

    const parseDate = d3.timeParse("%Y-%m-%dT%H:%M:%S%Z");
    data.forEach(d => {
        d.timestamp = parseDate(d.timestamp)?.toString() ?? "";
    });

    data = data.filter(d => !isNaN(d.score) && d.timestamp);
    const dates = data.map(d => new Date(d.timestamp));

    const x = d3.scaleTime()
        .domain([d3.min(dates)!, d3.max(dates) || new Date()])
        .range([0, width]);

    const maxYValue = d3.max(data, d => d.score) || 0;
    const y = d3.scaleLinear()
        .domain([0, maxYValue * 1.1])  // Add 10% padding above the maximum value
        .range([height, 0]);

    // X-axis with modern styling
    svg.append("g")
        .attr("transform", `translate(0,${height})`)
        .call(d3.axisBottom(x).tickSize(-height).tickPadding(10))
        .call(g => g.select(".domain").attr("stroke", "#e0e0e0").attr("stroke-width", 2))
        .call(g => g.selectAll(".tick text").attr("fill", "#666").attr("font-size", "12px"));

    // Y-axis with modern styling
    svg.append("g")
        .call(d3.axisLeft(y).tickSize(-width).tickPadding(10))
        .call(g => g.select(".domain").attr("stroke", "#e0e0e0").attr("stroke-width", 2))
        .call(g => g.selectAll(".tick text").attr("fill", "#666").attr("font-size", "12px"));

    // Modern grid lines
    svg.selectAll(".tick line")
        .attr("stroke", "#f0f0f0")
        .attr("stroke-opacity", 0.7)
        .attr("stroke-width", 1);

    // Line path for each team
    const line = d3.line<TeamScore>()
        .x(d => x(new Date(d.timestamp)))
        .y(d => y(d.score))
        .curve(d3.curveLinear);

    const teams = d3.group(data, d => d.team);

    teams.forEach((teamData, teamName) => {
        // Draw the line for each team with modern styling
        svg.append("path")
            .datum(teamData)
            .attr("fill", "none")
            .attr("stroke", colorScale(teamName)!)
            .attr("stroke-width", 3)
            .attr("stroke-linecap", "round")
            .attr("stroke-linejoin", "round")
            .attr("d", line);

        // Find the last data point for the team
        const lastDataPoint = teamData.reduce((latest, current) =>
            new Date(current.timestamp) > new Date(latest.timestamp) ? current : latest
        );

        // Draw a horizontal line extending from the last data point to the right edge of the graph
        svg.append("line")
            .attr("x1", x(new Date(lastDataPoint.timestamp)))  // Start at the last data point
            .attr("x2", width)  // Extend to the right edge of the graph
            .attr("y1", y(lastDataPoint.score))  // Y position based on the last score
            .attr("y2", y(lastDataPoint.score))  // Keep Y constant to form a horizontal line
            .attr("stroke", colorScale(teamName)!)  // Use the team's color for the line
            .attr("stroke-width", 2)
            .attr("stroke-opacity", 0.3)  // Lighter line using reduced opacity
            .attr("stroke-dasharray", "4,4");  // Dashed line for differentiation
    });

    // Tooltip container (reuse existing or create new)
    let tooltip = d3.select<HTMLDivElement, unknown>("body .tooltip-modern-timeline");
    if (tooltip.empty()) {
        tooltip = d3.select("body").append("div").attr("class", "tooltip-modern tooltip-modern-timeline").style("display", "none");
    }

    // Circles at data points with modern styling and hover effects
    const timeTooltipFormat = d3.timeFormat("%H:%M:%S");
    svg.selectAll("dot")
        .data(data)
        .enter()
        .append("circle")
        .attr("cx", d => x(new Date(d.timestamp)))
        .attr("cy", d => y(d.score))
        .attr("r", 6)
        .attr("fill", d => colorScale(d.team)!)
        .attr("stroke", "#fff")
        .attr("stroke-width", 2)
        .style("cursor", "pointer")
        .style("transition", "all 0.2s ease")
        .on("mouseover", function (event, d) {
            d3.select(this)
                .attr("r", 8)
                .attr("stroke-width", 3);
            const languageInfo = d.language ? `<br>Lang: ${d.language}` : '';
            tooltip.style("display", "block")
                .html(`Team: ${d.team}<br>Score: ${d.score}${languageInfo}<br>Time: ${timeTooltipFormat(new Date(d.timestamp))}`)
                .style("left", (event.pageX + 10) + "px")
                .style("top", (event.pageY - 20) + "px");
        })
        .on("mouseout", function () {
            d3.select(this)
                .attr("r", 6)
                .attr("stroke-width", 2);
            tooltip.style("display", "none");
        });
}

function fireConfetti() {
    if (typeof confetti !== "function") return;

    const duration = 2000;
    const animationEnd = Date.now() + duration;

    const defaults = {
        startVelocity: 45,
        spread: 360,
        ticks: 90,
        zIndex: 9999
    };

    function randomInRange(min: number, max: number) {
        return Math.random() * (max - min) + min;
    }

    const interval = setInterval(function() {
        const timeLeft = animationEnd - Date.now();

        if (timeLeft <= 0) {
            return clearInterval(interval);
        }

        const particleCount = 50 * (timeLeft / duration);

        confetti({
            ...defaults,
            particleCount,
            origin: { x: randomInRange(0.1, 0.3), y: Math.random() - 0.2 }
        });

        confetti({
            ...defaults,
            particleCount,
            origin: { x: randomInRange(0.7, 0.9), y: Math.random() - 0.2 }
        });

        confetti({
            ...defaults,
            particleCount,
            origin: { x: 0.5, y: -0.1 }
        });
    }, 200);
}

let partyInterval: number | null = null;

function party() {
    if (partyInterval !== null) return;

    partyInterval = window.setInterval(() => {
        fireConfetti();
    }, 2500);
}

// Expose the party function globally for manual triggering
;(window as any).party = party;

function handleTopTeamChange(latestScores: TeamScore[]) {
    if (!latestScores.length) return;

    const previousTopTeamKey = "previousTopTeam";
    const previousTopTeam = localStorage.getItem(previousTopTeamKey);
    const currentTopTeam = latestScores[0].team;

    if (previousTopTeam && currentTopTeam !== previousTopTeam) {
        fireConfetti();
    }

    localStorage.setItem(previousTopTeamKey, currentTopTeam);
}

function renderBarChart(data: TeamScore[]) {
    const margin = { top: 40, right: 30, bottom: 50, left: 50 };
    const width = document.body.clientWidth * 0.8 - margin.left - margin.right;
    const height = 400 - margin.top - margin.bottom;

    // Sort by timestamp first, then filter out old scores, keeping only the latest for each team
    const latestScores = Array.from(d3.group(data, d => d.team).values()).map(teamScores => {
        return teamScores.reduce((latest, current) => {
            return new Date(current.timestamp) > new Date(latest.timestamp) ? current : latest;
        });
    });

    // Sort the latest scores by score in descending order
    latestScores.sort((a, b) => b.score - a.score);

    handleTopTeamChange(latestScores);

    // Create a title for the graph
    d3.select("#bar-chart")
        .append("h2")
        .style("text-align", "center")
        .text("Latest Score");

    const svg = d3.select("#bar-chart")
        .append("svg")
        .attr("width", width + margin.left + margin.right)
        .attr("height", height + margin.top + margin.bottom)
        .append("g")
        .attr("transform", `translate(${margin.left},${margin.top})`);

    const y = d3.scaleBand()
        .domain(latestScores.map(d => d.team))
        .range([0, height])
        .padding(0.1);

    const maxYValue = d3.max(latestScores, d => d.score) || 0;
    const x = d3.scaleLinear()
        .domain([0, maxYValue * 1.2])  // Add 30% padding to leave space for team names
        .range([0, width]);

    // X-axis with modern styling
    svg.append("g")
        .attr("transform", `translate(0,${height})`)
        .call(d3.axisBottom(x).tickSize(-height).tickPadding(10))
        .call(g => g.select(".domain").attr("stroke", "#e0e0e0").attr("stroke-width", 2))
        .call(g => g.selectAll(".tick text").attr("fill", "#666").attr("font-size", "12px"));

    // Y-axis without labels (team names will be shown on the right of bars)
    svg.append("g")
        .call(d3.axisLeft(y).tickSize(-width).tickPadding(10).tickFormat(() => ""))
        .call(g => g.select(".domain").attr("stroke", "#e0e0e0").attr("stroke-width", 2));

    // Modern grid lines
    svg.selectAll(".tick line")
        .attr("stroke", "#f0f0f0")
        .attr("stroke-opacity", 0.7)
        .attr("stroke-width", 1);

    // Tooltip container (reuse existing or create new)
    let tooltip = d3.select<HTMLDivElement, unknown>("body .tooltip-modern-bar");
    if (tooltip.empty()) {
        tooltip = d3.select("body").append("div").attr("class", "tooltip-modern tooltip-modern-bar").style("display", "none");
    }

    svg.selectAll(".bar")
        .data(latestScores)
        .enter()
        .append("rect")
        .attr("x", 0)
        .attr("y", d => y(d.team)!)
        .attr("width", d => x(d.score))
        .attr("height", y.bandwidth())
        .attr("fill", d => colorScale(d.team)!)
        .attr("rx", 6)
        .attr("ry", 6)
        .style("cursor", "pointer")
        .style("opacity", 0.85)
        .on("mouseover", function (event, d) {
            d3.select(this).style("opacity", 1);
            const languageInfo = d.language ? `<br>Lang: ${d.language}` : '';
            tooltip.style("display", "block")
                .html(`Team: ${d.team}<br>Score: ${d.score}${languageInfo}`)
                .style("left", (event.pageX + 10) + "px")
                .style("top", (event.pageY - 20) + "px");
        })
        .on("mouseout", function () {
            d3.select(this).style("opacity", 0.85);
            tooltip.style("display", "none");
        });

    // Add team names to the right of each bar
    svg.selectAll(".label")
        .data(latestScores)
        .enter()
        .append("text")
        .attr("class", "label")
        .attr("x", d => x(d.score) + 10)
        .attr("y", d => y(d.team)! + y.bandwidth() / 2)
        .attr("dy", "0.35em")
        .style("fill", "#333")
        .text(d => d.team);
}

function showFinalCountdown() {
    const DURATION_MS = 60 * 60 * 1000; // 1 hour
    const LAST_10_MIN_MS = 10 * 60 * 1000;

    const now = Date.now();
    const stored = localStorage.getItem(countdownKey);
    const start = stored ? parseInt(stored) || now : now;

    if (!stored) {
        localStorage.setItem(countdownKey, String(start));
    }

    // Clear everything and render only countdown
    const body = d3.select("body");
    body.selectAll("*").remove();

    body.style("margin", "0")
        .style("height", "100vh")
        .style("display", "flex")
        .style("flex-direction", "column")
        .style("align-items", "center")
        .style("justify-content", "center")
        .style("background-color", "#f4f4f9");

    body.append("h1")
        .text("Final Countdown")
        .style("text-align", "center")
        .style("margin", "0 0 16px 0")
        .style("font-family", "Segoe UI, Tahoma, Geneva, Verdana, sans-serif");

    const timeEl = body.append("div")
        .attr("id", "countdown")
        .style("font-size", "64px")
        .style("text-align", "center")
        .style("font-family", "monospace")
        .style("font-weight", "bold")
        .style("color", "#111827");

    if (countdownTimerId !== null) {
        clearInterval(countdownTimerId);
    }

    const tick = () => {
        const elapsed = Date.now() - start;
        const remaining = DURATION_MS - elapsed;

        if (remaining <= 0) {
            timeEl
                .text("00:00:00")
                .style("color", "red");

            // Call party when scoreboard re-open
            localStorage.setItem(partyKey, "1");

            clearInterval(countdownTimerId!);
            countdownTimerId = null;
            return;
        }

        const totalSeconds = Math.floor(remaining / 1000);
        const h = Math.floor(totalSeconds / 3600);
        const m = Math.floor((totalSeconds % 3600) / 60);
        const s = totalSeconds % 60;

        timeEl.text(
            `${h.toString().padStart(2, "0")}:` +
            `${m.toString().padStart(2, "0")}:` +
            `${s.toString().padStart(2, "0")}`
        );

        // Turn red in the last 10 minutes
        if (remaining <= LAST_10_MIN_MS) {
            timeEl.style("color", "#FF3838");
        } else {
            timeEl.style("color", "#ffffffff");
        }
    };

    tick();
    countdownTimerId = setInterval(tick, 1000);
}


// Function to update the graph
function updateGraph() {
    fetchData().then(data => {
        if (!data || data.length === 0) {
            return;
        }

        // Clear existing graphs
        d3.select("#timeline-chart").selectAll("*").remove();
        d3.select("#bar-chart").selectAll("*").remove();

        // Re-render the graphs with the latest data
        renderTimeline(data);
        renderBarChart(data);
    });
}

// Initial graph rendering
updateGraph();

// Fetch and refresh the graph every 30 seconds (30,000 ms)
setInterval(updateGraph, 30000);
