from __future__ import annotations

from math import ceil, log2, sqrt
from typing import Sequence

from qiskit import QuantumCircuit, transpile
from qiskit.providers.basic_provider import BasicProvider

_backend = BasicProvider().get_backend("basic_simulator")


def _sample_bitstring(circuit: "QuantumCircuit") -> str:
    compiled = transpile(circuit, _backend)
    result = _backend.run(compiled, shots=1).result()
    counts = result.get_counts()
    return next(iter(counts))


def sample_superposition_index() -> int:
    qc = QuantumCircuit(1, 1)
    qc.h(0)
    qc.measure(0, 0)
    bitstring = _sample_bitstring(qc)
    return int(bitstring, 2)


def sample_weighted_index(probabilities: Sequence[float]) -> int:
    probs = [float(p) for p in probabilities]
    if not probs:
        raise ValueError("probabilities must not be empty")

    total = sum(max(0.0, p) for p in probs)
    if total <= 0:
        raise ValueError("probabilities must have positive mass")

    normalized = [max(0.0, p) / total for p in probs]

    n_outcomes = len(normalized)
    n_qubits = max(1, ceil(log2(n_outcomes)))
    basis_size = 2**n_qubits

    amplitudes = [0.0] * basis_size
    for i, p in enumerate(normalized):
        amplitudes[i] = sqrt(p)

    qc = QuantumCircuit(n_qubits, n_qubits)
    qc.initialize(amplitudes, list(range(n_qubits)))
    qc.measure(list(range(n_qubits)), list(range(n_qubits)))

    for _ in range(8):
        bitstring = _sample_bitstring(qc)
        idx = int(bitstring, 2)
        if idx < n_outcomes:
            return idx

    raise RuntimeError("Failed to sample a valid quantum outcome.")
