#!/bin/bash

# Ensure we are running Bash 4.0 or higher for associative arrays
if ((BASH_VERSINFO[0] < 4)); then
    echo "Error: This script requires Bash 4.0 or higher." >&2
    exit 1
fi

# ==========================================
# 0. Argument Parsing & Usage
# ==========================================

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -m, --machine-type <type>  Required. The GCE machine type (e.g., n2d-standard-2)."
    echo "  -c, --confidential         Optional. Set if GCE instance has confidential computing enabled."
    echo "  -h, --help                 Show this help message."
    echo ""
    echo "Script Examples:"
    echo "  $0 --machine-type n2d-standard-2 --confidential"
    echo "  $0 -m c3-standard-4"
    echo ""
    echo "Kubernetes Integration:"
    echo "  To apply generated labels directly to a node:"
    echo "  kubectl label nodes <node-name> \$($0 -m <machine-type>)"
    exit 1
}

MACHINE_TYPE=""
IS_CONFIDENTIAL="false"

while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
        -m|--machine-type)
            MACHINE_TYPE="$2"
            shift # past argument
            shift # past value
            ;;
        -c|--confidential)
            IS_CONFIDENTIAL="true"
            shift # past argument
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "Error: Unknown option '$1'"
            usage
            ;;
    esac
done

# Validate required arguments
if [[ -z "$MACHINE_TYPE" ]]; then
    echo "Error: --machine-type argument is required."
    usage
fi

# ==========================================
# 1. Define Constants (Disk Types)
# ==========================================
PD_STANDARD="pd-standard"
PD_BALANCED="pd-balanced"
PD_SSD="pd-ssd"
PD_EXTREME="pd-extreme"
HYPERDISK_BALANCED="hyperdisk-balanced"
HYPERDISK_EXTREME="hyperdisk-extreme"
HYPERDISK_THROUGHPUT="hyperdisk-throughput"
HYPERDISK_BALANCED_HA="hyperdisk-balanced-high-availability"
HYPERDISK_ML="hyperdisk-ml"

# ==========================================
# 2. Define Modes (Confidential Enum)
# ==========================================
MODE_CONF_ONLY="ConfidentialOnly"
MODE_NON_CONF_ONLY="NonConfidentialOnly"
MODE_UNSPECIFIED="Unspecified"

# ==========================================
# 3. Source of Truth (SOT) Data
# ==========================================
# We use an associative array with composite keys: "FAMILY:DISK_TYPE"
declare -A SOT

# --- a2 ---
SOT["a2:$HYPERDISK_BALANCED"]=$MODE_CONF_ONLY
SOT["a2:$HYPERDISK_ML"]=$MODE_NON_CONF_ONLY
SOT["a2:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["a2:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["a2:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["a2:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- a3 ---
SOT["a3:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["a3:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY
SOT["a3:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["a3:$HYPERDISK_THROUGHPUT"]=$MODE_NON_CONF_ONLY
SOT["a3:$HYPERDISK_ML"]=$MODE_NON_CONF_ONLY
SOT["a3:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["a3:$PD_SSD"]=$MODE_UNSPECIFIED

# --- a4 ---
SOT["a4:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["a4:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["a4:$HYPERDISK_ML"]=$MODE_UNSPECIFIED

# --- a4x ---
SOT["a4x:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["a4x:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["a4x:$HYPERDISK_ML"]=$MODE_UNSPECIFIED

# --- c2 ---
SOT["c2:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["c2:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["c2:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["c2:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- c2d ---
SOT["c2d:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["c2d:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["c2d:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["c2d:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- c3 ---
SOT["c3:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["c3:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["c3:$HYPERDISK_THROUGHPUT"]=$MODE_NON_CONF_ONLY
SOT["c3:$HYPERDISK_ML"]=$MODE_NON_CONF_ONLY
SOT["c3:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY
SOT["c3:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["c3:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["c3:$PD_EXTREME"]=$MODE_UNSPECIFIED

# --- c3d ---
SOT["c3d:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["c3d:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["c3d:$HYPERDISK_ML"]=$MODE_NON_CONF_ONLY
SOT["c3d:$HYPERDISK_THROUGHPUT"]=$MODE_NON_CONF_ONLY
SOT["c3d:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["c3d:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["c3d:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY

# --- c4 ---
SOT["c4:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["c4:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY
SOT["c4:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["c4:$HYPERDISK_THROUGHPUT"]=$MODE_NON_CONF_ONLY

# --- c4a ---
SOT["c4a:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["c4a:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["c4a:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY
SOT["c4a:$HYPERDISK_THROUGHPUT"]=$MODE_NON_CONF_ONLY
SOT["c4a:$HYPERDISK_ML"]=$MODE_UNSPECIFIED
SOT["c4a:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["c4a:$PD_SSD"]=$MODE_UNSPECIFIED

# --- c4d ---
SOT["c4d:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["c4d:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["c4d:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY

# --- ct3 ---
SOT["ct3:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["ct3:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["ct3:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- ct3p ---
SOT["ct3p:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["ct3p:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["ct3p:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- ct4l ---
SOT["ct4l:$HYPERDISK_BALANCED"]=$MODE_CONF_ONLY
SOT["ct4l:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["ct4l:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["ct4l:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["ct4l:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- ct4p ---
SOT["ct4p:$HYPERDISK_BALANCED"]=$MODE_CONF_ONLY
SOT["ct4p:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["ct4p:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["ct4p:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["ct4p:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- ct5l ---
SOT["ct5l:$HYPERDISK_BALANCED"]=$MODE_CONF_ONLY
SOT["ct5l:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["ct5l:$PD_SSD"]=$MODE_UNSPECIFIED

# --- ct5lp ---
SOT["ct5lp:$HYPERDISK_BALANCED"]=$MODE_CONF_ONLY
SOT["ct5lp:$HYPERDISK_ML"]=$MODE_NON_CONF_ONLY
SOT["ct5lp:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["ct5lp:$PD_SSD"]=$MODE_UNSPECIFIED

# --- ct5p ---
SOT["ct5p:$HYPERDISK_BALANCED"]=$MODE_CONF_ONLY
SOT["ct5p:$HYPERDISK_ML"]=$MODE_NON_CONF_ONLY
SOT["ct5p:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["ct5p:$PD_SSD"]=$MODE_UNSPECIFIED

# --- ct6e ---
SOT["ct6e:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["ct6e:$HYPERDISK_ML"]=$MODE_NON_CONF_ONLY

# --- e2 ---
SOT["e2:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["e2:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["e2:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["e2:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- ek ---
SOT["ek:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["ek:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["ek:$PD_STANDARD"]=$MODE_UNSPECIFIED
SOT["ek:$PD_EXTREME"]=$MODE_UNSPECIFIED

# --- g2 ---
SOT["g2:$HYPERDISK_THROUGHPUT"]=$MODE_UNSPECIFIED
SOT["g2:$HYPERDISK_ML"]=$MODE_NON_CONF_ONLY
SOT["g2:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["g2:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["g2:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- g4 ---
SOT["g4:$HYPERDISK_BALANCED"]=$MODE_UNSPECIFIED
SOT["g4:$HYPERDISK_BALANCED_HA"]=$MODE_UNSPECIFIED
SOT["g4:$HYPERDISK_EXTREME"]=$MODE_UNSPECIFIED
SOT["g4:$HYPERDISK_THROUGHPUT"]=$MODE_UNSPECIFIED
SOT["g4:$HYPERDISK_ML"]=$MODE_UNSPECIFIED

# --- h3 ---
SOT["h3:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["h3:$HYPERDISK_THROUGHPUT"]=$MODE_UNSPECIFIED
SOT["h3:$PD_BALANCED"]=$MODE_UNSPECIFIED

# --- h4d ---
SOT["h4d:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["h4d:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY

# --- m1 ---
SOT["m1:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["m1:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["m1:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["m1:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["m1:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["m1:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- m2 ---
SOT["m2:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["m2:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["m2:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["m2:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["m2:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["m2:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- m3 ---
SOT["m3:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["m3:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["m3:$HYPERDISK_THROUGHPUT"]=$MODE_NON_CONF_ONLY
SOT["m3:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY
SOT["m3:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["m3:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["m3:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["m3:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- m4 ---
SOT["m4:$HYPERDISK_BALANCED"]=$MODE_CONF_ONLY
SOT["m4:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY

# --- n1 ---
SOT["n1:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["n1:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["n1:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["n1:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- n2 ---
SOT["n2:$HYPERDISK_BALANCED"]=$MODE_CONF_ONLY
SOT["n2:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["n2:$HYPERDISK_ML"]=$MODE_UNSPECIFIED
SOT["n2:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["n2:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["n2:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["n2:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- n2d ---
SOT["n2d:$HYPERDISK_BALANCED"]=$MODE_CONF_ONLY
SOT["n2d:$HYPERDISK_THROUGHPUT"]=$MODE_NON_CONF_ONLY
SOT["n2d:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["n2d:$PD_EXTREME"]=$MODE_UNSPECIFIED
SOT["n2d:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["n2d:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- n4 ---
SOT["n4:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["n4:$HYPERDISK_THROUGHPUT"]=$MODE_NON_CONF_ONLY
SOT["n4:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY
SOT["n4:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["n4:$PD_SSD"]=$MODE_UNSPECIFIED

# --- t2a ---
SOT["t2a:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["t2a:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["t2a:$PD_STANDARD"]=$MODE_UNSPECIFIED
SOT["t2a:$PD_EXTREME"]=$MODE_UNSPECIFIED

# --- t2d ---
SOT["t2d:$HYPERDISK_THROUGHPUT"]=$MODE_NON_CONF_ONLY
SOT["t2d:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["t2d:$PD_SSD"]=$MODE_UNSPECIFIED
SOT["t2d:$PD_STANDARD"]=$MODE_UNSPECIFIED

# --- tpu7x ---
SOT["tpu7x:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["tpu7x:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["tpu7x:$HYPERDISK_THROUGHPUT"]=$MODE_UNSPECIFIED
SOT["tpu7x:$HYPERDISK_ML"]=$MODE_UNSPECIFIED

# --- z3 ---
SOT["z3:$HYPERDISK_EXTREME"]=$MODE_NON_CONF_ONLY
SOT["z3:$HYPERDISK_THROUGHPUT"]=$MODE_UNSPECIFIED
SOT["z3:$HYPERDISK_BALANCED"]=$MODE_NON_CONF_ONLY
SOT["z3:$HYPERDISK_BALANCED_HA"]=$MODE_NON_CONF_ONLY
SOT["z3:$PD_BALANCED"]=$MODE_UNSPECIFIED
SOT["z3:$PD_SSD"]=$MODE_UNSPECIFIED

# ==========================================
# 4. Logic Function
# ==========================================

# Usage: list_possible_attached_disks <family> <is_confidential (true|false)>
list_possible_attached_disks() {
    local family="$1"
    local is_confidential="$2"
    local family_found=false
    local supported_disks=()

    # Iterate through all keys in SOT to find matches for the family.
    # (Bash does not support direct nested map lookups nicely, so we filter keys)
    for key in "${!SOT[@]}"; do
        # Check if key starts with "family:"
        if [[ "$key" == "$family":* ]]; then
            family_found=true

            # Extract disk type and mode
            local disk_type="${key#*:}"
            local mode="${SOT[$key]}"

            # Logic: Filter based on confidentiality
            if [[ "$mode" == "$MODE_NON_CONF_ONLY" && "$is_confidential" == "true" ]]; then
                continue
            elif [[ "$mode" == "$MODE_CONF_ONLY" && "$is_confidential" == "false" ]]; then
                continue
            else
                supported_disks+=("$disk_type")
            fi
        fi
    done

    if [[ "$family_found" == "false" ]]; then
        echo "Error: family '$family' not found in SOT" >&2
        return 1
    fi

    # Output the list
    echo "${supported_disks[@]}"
}

# ==========================================
# 5. Main Execution
# ==========================================

FAMILY_ARG="${MACHINE_TYPE%%-*}"
RESULT=$(list_possible_attached_disks "$FAMILY_ARG" "$IS_CONFIDENTIAL")

if [[ $? -eq 0 ]]; then
    for disk in $RESULT; do
        echo "disk-type.gke.io/$disk=\"true\""
    done
else
    # The error message is already printed to stderr by the function
    exit 1
fi