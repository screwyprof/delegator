# Development Log

Development decisions, discoveries, and learning process during implementation.

## Environment & Tooling Decisions

**Go 1.24 + Nix Decision**: Modern tooling for development convenience
- **Go 1.24**: Latest stable Go release with improved performance and features
- **Nix**: Reproducible development environment - no "works on my machine" issues
- **Benefit**: Deterministic builds, automatic tool management, zero setup friction

**Go Workspace Over Monorepo**: Independent modules within shared workspace
- **Problem**: Need service independence but shared tooling
- **Solution**: Go workspace - each service has own go.mod but shared build tools
- **Result**: Can version/deploy services independently while keeping DX simple

## Tzkt API Client Development Journey

### Initial Research
**Problem**: How to efficiently fetch delegation data from Tzkt API?

**Starting Point**: Well-documented Tzkt API with rich filtering capabilities
- **Endpoint**: `GET https://api.tzkt.io/v1/operations/delegations`
- **Challenge**: Large response payloads, need to optimize bandwidth

### Bandwidth Optimization Investigation

**Experiment 1**: Field selection
- **Test**: Compare full response vs selective fields
- **Result**: `select=id,timestamp,amount,sender,level` → 67% bandwidth reduction (889→293 bytes)
- **Learning**: API supports precise field selection, massive savings

**Experiment 2**: Compression behavior
- **Test**: Various request sizes, observing when Tzkt compresses responses
- **Findings**:
  - Small requests (≤5 records): Usually uncompressed
  - Large requests (≥500 records): Always gzip-compressed  
  - Threshold: Compression kicks in around 6-10 records
- **Learning**: Go's HTTP client handles compression automatically, no manual work needed

**Experiment 3**: Pagination strategies
- **Traditional offset**: `offset=100&limit=10` - requires maintaining state
- **ID-based**: `id.gt=123456789&limit=10` - stateless, no drift
- **Decision**: Use ID-based pagination for checkpointing simplicity

### Live API Testing Results
**Test Period**: 24 hours of real delegation monitoring
**Observations**:
- **Rate**: ~5-6 delegations per hour (132 total) - normal Tezos activity
- **Reliability**: Zero API failures or rate limits encountered
- **Performance**: Field selection + gzip works as expected

### Final Client Implementation
**Interface**: Simple, focused on delegations only
```go
GetDelegations(ctx context.Context, req DelegationsRequest) ([]Delegation, error)
```

**Built-in Optimizations**:
- Automatic compression via Go's HTTP client (transparent gzip handling)
- Field selection for bandwidth efficiency
- ID-based pagination support

**Current Status**: ✅ Complete and tested, ready for scraper integration

**Deliberate Limitations**: No retry logic, circuit breakers, or rate limiting - keeping it simple. Production concerns can be handled at scraper level.

### Current Status  
**Completed**: Tzkt client with proven optimizations, ready for scraper integration
**Next**: Implement scraper service using the validated client and learnings

