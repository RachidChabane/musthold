"""The public results site: static pages rendered from the sealed results of a run, or from the synthetic fixtures.

Every module here is a pure function of its inputs; reading the run directory and writing the pages belong to
the adapters, and the command line wires them.
"""
