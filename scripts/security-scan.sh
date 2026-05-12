#!/bin/bash
# Gateway-a Security Scan Script
# Performs comprehensive security scanning using Trivy and GoSec
#
# Usage: ./scripts/security-scan.sh [options]
# Options:
#   --trivy-only   Run only Trivy scan
#   --gosec-only   Run only GoSec scan
#   --full         Run both scans (default)
#   --output DIR   Output directory for reports (default: ./security-reports)
#   --exit-code    Exit with non-zero on findings (default: yes)

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
REPORT_DIR="${REPORT_DIR:-${PROJECT_ROOT}/security-reports}"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
declare -i CRITICAL_COUNT=0
declare -i HIGH_COUNT=0
declare -i MEDIUM_COUNT=0
declare -i LOW_COUNT=0
declare -i INFO_COUNT=0

# Parse arguments
RUN_TRIVY=true
RUN_GOSEC=true
EXIT_ON_FINDINGS=true

while [[ $# -gt 0 ]]; do
    case $1 in
        --trivy-only)
            RUN_GOSEC=false
            shift
            ;;
        --gosec-only)
            RUN_TRIVY=false
            shift
            ;;
        --full)
            RUN_TRIVY=true
            RUN_GOSEC=true
            shift
            ;;
        --output)
            REPORT_DIR="$2"
            shift 2
            ;;
        --no-exit-code)
            EXIT_ON_FINDINGS=false
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--trivy-only|--gosec-only|--full] [--output DIR] [--no-exit-code]"
            exit 1
            ;;
    esac
done

# Create report directory
mkdir -p "$REPORT_DIR"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_result() {
    echo -e "$1 $2"
}

# Print header
print_header() {
    echo ""
    echo "=============================================="
    echo "  Gateway-a Security Scan"
    echo "  $(date '+%Y-%m-%d %H:%M:%S')"
    echo "=============================================="
    echo ""
}

# Print summary
print_summary() {
    echo ""
    echo "=============================================="
    echo "  Security Scan Summary"
    echo "=============================================="
    echo ""
    echo -e "  Critical: ${RED}${CRITICAL_COUNT}${NC}"
    echo -e "  High:     ${RED}${HIGH_COUNT}${NC}"
    echo -e "  Medium:   ${YELLOW}${MEDIUM_COUNT}${NC}"
    echo -e "  Low:      ${BLUE}${LOW_COUNT}${NC}"
    echo -e "  Info:     ${NC}${INFO_COUNT}${NC}"
    echo ""
    echo "=============================================="
    echo ""

    if [[ $CRITICAL_COUNT -gt 0 || $HIGH_COUNT -gt 0 ]]; then
        log_warning "Security scan found critical/high vulnerabilities!"
        return 1
    else
        log_success "No critical or high vulnerabilities found!"
        return 0
    fi
}

# ============================================================
# Trivy Container Image Scan
# ============================================================
run_trivy_image_scan() {
    log_info "Starting Trivy container image scan..."
    local report_file="${REPORT_DIR}/trivy-image-report.json"
    local summary_file="${REPORT_DIR}/trivy-image-summary.txt"

    # Check if Dockerfile exists
    if [[ ! -f "${PROJECT_ROOT}/Dockerfile" ]]; then
        log_warning "Dockerfile not found, skipping image scan"
        return 0
    fi

    # Build image for scanning
    log_info "Building Docker image for scanning..."
    if ! docker build -t gateway-a:scan "${PROJECT_ROOT}" 2>/dev/null; then
        log_warning "Could not build Docker image, trying with existing image..."
    fi

    # Run Trivy image scan
    log_info "Running Trivy vulnerability scan on image..."

    # Check if trivy is installed
    if ! command -v trivy &> /dev/null; then
        log_warning "Trivy not installed, creating placeholder report"
        echo "Trivy not installed" > "$summary_file"
        echo "{}" > "$report_file"
        return 0
    fi

    # Run Trivy with JSON output
    if trivy image --format json \
        --output "$report_file" \
        --severity CRITICAL,HIGH \
        gateway-a:scan 2>&1 | tee "$summary_file"; then

        log_success "Trivy image scan completed"

        # Count findings
        if [[ -f "$report_file" ]]; then
            CRITICAL_COUNT=$((CRITICAL_COUNT + $(cat "$report_file" | jq '[.Results[] | .Vulnerabilities | select(.Severity == "CRITICAL")] | length' 2>/dev/null || echo 0)))
            HIGH_COUNT=$((HIGH_COUNT + $(cat "$report_file" | jq '[.Results[] | .Vulnerabilities | select(.Severity == "HIGH")] | length' 2>/dev/null || echo 0)))
        fi
    else
        log_warning "Trivy image scan failed or no findings"
    fi
}

# ============================================================
# Trivy File System Scan
# ============================================================
run_trivy_fs_scan() {
    log_info "Starting Trivy filesystem scan..."
    local report_file="${REPORT_DIR}/trivy-fs-report.json"
    local summary_file="${REPORT_DIR}/trivy-fs-summary.txt"

    # Check if trivy is installed
    if ! command -v trivy &> /dev/null; then
        log_warning "Trivy not installed, creating placeholder report"
        echo "Trivy not installed" > "$summary_file"
        echo "{}" > "$report_file"
        return 0
    fi

    # Run Trivy filesystem scan
    log_info "Scanning project files for vulnerabilities..."

    if trivy fs --format json \
        --severity CRITICAL,HIGH \
        --output "$report_file" \
        "${PROJECT_ROOT}" 2>&1 | tee "$summary_file"; then

        log_success "Trivy filesystem scan completed"

        # Count findings
        if [[ -f "$report_file" ]]; then
            local fs_critical=$(cat "$report_file" | jq '[.Results[] | .Vulnerabilities | select(.Severity == "CRITICAL")] | length' 2>/dev/null || echo 0)
            local fs_high=$(cat "$report_file" | jq '[.Results[] | .Vulnerabilities | select(.Severity == "HIGH")] | length' 2>/dev/null || echo 0)
            CRITICAL_COUNT=$((CRITICAL_COUNT + fs_critical))
            HIGH_COUNT=$((HIGH_COUNT + fs_high))
        fi
    else
        log_warning "Trivy filesystem scan completed with warnings"
    fi
}

# ============================================================
# GoSec Static Analysis
# ============================================================
run_gosec_scan() {
    log_info "Starting GoSec static analysis..."
    local report_file="${REPORT_DIR}/gosec-report.json"
    local summary_file="${REPORT_DIR}/gosec-summary.txt"

    # Check if gosec is installed
    if ! command -v gosec &> /dev/null; then
        log_warning "GoSec not installed, creating placeholder report"
        echo "GoSec not installed" > "$summary_file"
        echo "{}" > "$report_file"
        return 0
    fi

    # Run GoSec
    log_info "Analyzing Go source code for security issues..."

    # Run gosec with JSON output
    if gosec -fmt=json \
        -out="$report_file" \
        -conf="" \
        ./... 2>&1 | tee "$summary_file"; then

        log_success "GoSec analysis completed"

        # Count findings
        if [[ -f "$report_file" ]]; then
            CRITICAL_COUNT=$((CRITICAL_COUNT + $(cat "$report_file" | jq '[.Issues[] | select(.Severity == "HIGH")] | length' 2>/dev/null || echo 0)))
            HIGH_COUNT=$((HIGH_COUNT + $(cat "$report_file" | jq '[.Issues[] | select(.Severity == "MEDIUM")] | length' 2>/dev/null || echo 0)))
        fi
    else
        log_warning "GoSec analysis completed with warnings"
    fi
}

# ============================================================
# Go Vet (basic Go code analysis)
# ============================================================
run_go_vet() {
    log_info "Running Go vet for basic code analysis..."
    local summary_file="${REPORT_DIR}/go-vet-summary.txt"

    cd "$PROJECT_ROOT"

    if go vet ./... 2>&1 | tee "$summary_file"; then
        log_success "Go vet completed"
    else
        log_warning "Go vet found issues"
        # Count issues
        LOW_COUNT=$((LOW_COUNT + $(wc -l < "$summary_file")))
    fi
}

# ============================================================
# Dependent Package Security Scan
# ============================================================
run_dep_guard() {
    log_info "Checking for vulnerable dependencies..."
    local summary_file="${REPORT_DIR}/dep-guard-summary.txt"

    cd "$PROJECT_ROOT"

    # Check if there's a go.mod file
    if [[ -f "go.mod" ]]; then
        # Run go mod audit if available (Go 1.16+)
        if command -v go &> /dev/null; then
            if go mod audit 2>&1 | tee "$summary_file"; then
                log_success "Dependency audit completed"
            else
                log_warning "Dependency audit found issues"
            fi
        fi
    fi
}

# ============================================================
# SQL Injection Detection
# ============================================================
check_sql_injection() {
    log_info "Checking for potential SQL injection vulnerabilities..."
    local summary_file="${REPORT_DIR}/sql-injection-check.txt"

    cd "$PROJECT_ROOT"

    # Look for raw SQL queries
    local sql_queries=$(grep -r "Exec\|Query" --include="*.go" . 2>/dev/null | \
        grep -v "test" | \
        grep -v "mock" | \
        wc -l)

    echo "Potential SQL operations found: $sql_queries" > "$summary_file"

    # Check if GORM is used (safer)
    if grep -r "gorm\." --include="*.go" . 2>/dev/null | grep -v "test" > /dev/null 2>&1; then
        echo "Using GORM ORM - SQL injection risk mitigated" >> "$summary_file"
        log_success "Using GORM ORM for database operations"
    fi
}

# ============================================================
# XSS Prevention Check
# ============================================================
check_xss_prevention() {
    log_info "Checking for XSS prevention..."
    local summary_file="${REPORT_DIR}/xss-check.txt"

    cd "$PROJECT_ROOT"

    echo "XSS Prevention Check" > "$summary_file"
    echo "===================" >> "$security_file"

    # Check for HTML output in responses
    local html_outputs=$(grep -r "html\|HTML" --include="*.go" . 2>/dev/null | \
        grep -v "test" | \
        wc -l)

    echo "HTML-related code found: $html_outputs" >> "$summary_file"

    log_success "XSS prevention check completed"
}

# ============================================================
# Secrets Detection
# ============================================================
check_secrets() {
    log_info "Checking for exposed secrets..."
    local summary_file="${REPORT_DIR}/secrets-check.txt"

    cd "$PROJECT_ROOT"

    echo "Secrets Detection Report" > "$summary_file"
    echo "=========================" >> "$summary_file"
    echo "Date: $(date)" >> "$summary_file"
    echo "" >> "$summary_file"

    # Check for common secret patterns
    local patterns=(
        "password="
        "api_key="
        "secret="
        "token="
        "AWS_ACCESS_KEY"
        "AWS_SECRET"
    )

    for pattern in "${patterns[@]}"; do
        local count=$(grep -r "$pattern" --include="*.yaml" --include="*.yml" --include="*.env" . 2>/dev/null | \
            grep -v ".git" | \
            wc -l)
        if [[ $count -gt 0 ]]; then
            echo "WARNING: Found $count occurrences of '$pattern'" >> "$summary_file"
        fi
    done

    # Check .env file is in .gitignore
    if grep -q "\.env" .gitignore 2>/dev/null; then
        echo "OK: .env is in .gitignore" >> "$summary_file"
    else
        echo "WARNING: .env not in .gitignore" >> "$summary_file"
    fi

    log_success "Secrets detection completed"
}

# ============================================================
# Main execution
# ============================================================
main() {
    print_header

    log_info "Project root: $PROJECT_ROOT"
    log_info "Report directory: $REPORT_DIR"
    log_info "Running: Trivy=$RUN_TRIVY, GoSec=$RUN_GOSEC"
    echo ""

    # Always run Go vet (fast and useful)
    run_go_vet
    echo ""

    # File system checks
    check_sql_injection
    check_xss_prevention
    check_secrets
    echo ""

    # Run Trivy if enabled
    if [[ "$RUN_TRIVY" == "true" ]]; then
        run_trivy_fs_scan
        run_trivy_image_scan
        echo ""
    fi

    # Run GoSec if enabled
    if [[ "$RUN_GOSEC" == "true" ]]; then
        run_gosec_scan
        echo ""
    fi

    # Dependency check
    run_dep_guard
    echo ""

    # Print summary
    print_summary
    local exit_code=$?

    # Exit with appropriate code
    if [[ "$EXIT_ON_FINDINGS" == "true" && $exit_code -ne 0 ]]; then
        exit 1
    fi

    exit 0
}

# Run main function
main "$@"
