package service

import (
	"sort"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
)

// 蝴蝶图：双向 BFS 与统计口径。

func (s *Service) Fan(dimID int, qn, granularity string, depth int, types []string, hideImplDetail bool) (dto.FanPayload, error) {
	if granularity == "method" {
		return s.fanMethod(dimID, qn, depth, types, hideImplDetail)
	}
	return s.fanType(dimID, qn, granularity, depth, types)
}

func (s *Service) FanStats(dimID int) ([]dto.FanStatResp, error) {
	fans, err := s.repo.TypeFans(dimID)
	if err != nil {
		return nil, err
	}
	pkgOf := s.repo.TypeToPackage(dimID)
	out := make([]dto.FanStatResp, 0, len(fans))
	for qn, f := range fans {
		out = append(out, dto.FanStatResp{
			QN: qn, Pkg: pkgOf[qn],
			FanIn: f.FanIn, FanOut: f.FanOut,
			FanInTypes: f.FanInTypes, FanOutTypes: f.FanOutTypes,
			FanInSites: f.FanInSites, FanOutSites: f.FanOutSites,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].QN < out[j].QN })
	return out, nil
}

func (s *Service) fanType(dimID int, qn, granularity string, depth int, types []string) (dto.FanPayload, error) {
	edges, err := s.repo.TypeEdges(dimID, types)
	if err != nil {
		return dto.FanPayload{}, err
	}
	pkgOf := s.repo.TypeToPackage(dimID)
	// 包粒度：把类型级边按直接所属包上卷（目标类 QN 也映射到包）。
	if granularity == "package" {
		agg := map[string]*dto.FanEdge{}
		for _, e := range edges {
			sp, tp := pkgOf[e.Source], pkgOf[e.Target]
			if sp == "" || tp == "" || sp == tp {
				continue
			}
			key := sp + "\x00" + tp
			if hit, ok := agg[key]; ok {
				hit.Count += e.Count
				hit.Types = mergeStrings(hit.Types, e.Types)
			} else {
				agg[key] = &dto.FanEdge{Source: sp, Target: tp, Types: e.Types, Count: e.Count, Type: firstOr(e.Types)}
			}
		}
		edges = edges[:0]
		for _, v := range agg {
			edges = append(edges, repository.Edge{Source: v.Source, Target: v.Target, Types: v.Types, Count: v.Count, Type: v.Type})
		}
		sort.Slice(edges, func(i, j int) bool {
			if edges[i].Source != edges[j].Source {
				return edges[i].Source < edges[j].Source
			}
			return edges[i].Target < edges[j].Target
		})
		if p := pkgOf[qn]; p != "" {
			qn = p
		}
	}

	fans, err := s.repo.TypeFans(dimID)
	if err != nil {
		return dto.FanPayload{}, err
	}

	crossPkgOf := func(a, b string) bool {
		if granularity != "type" {
			return false
		}
		pa, pb := pkgOf[a], pkgOf[b]
		return pa != "" && pb != "" && pa != pb
	}

	// 双向 BFS：上游（谁依赖我）与下游（我依赖谁）。
	upAdj := map[string][]repository.Edge{}
	downAdj := map[string][]repository.Edge{}
	for _, e := range edges {
		downAdj[e.Source] = append(downAdj[e.Source], e)
		upAdj[e.Target] = append(upAdj[e.Target], e)
	}

	nodes := map[string]*dto.FanNode{}
	addNode := func(id, side string, d int) {
		if _, ok := nodes[id]; ok {
			return
		}
		n := &dto.FanNode{ID: id, Label: lastSeg(id), Side: side, Depth: d, Kind: s.repo.KindOf(dimID, id)}
		if f, ok := fans[id]; ok {
			n.FanIn, n.FanOut = f.FanIn, f.FanOut
		}
		nodes[id] = n
	}
	addNode(qn, "center", 0)

	upDepth, downDepth := map[string]int{}, map[string]int{}
	frontier := []string{qn}
	for d := 1; d <= depth; d++ {
		var next []string
		for _, f := range frontier {
			for _, e := range upAdj[f] {
				if _, ok := nodes[e.Source]; !ok {
					addNode(e.Source, "up", d)
					upDepth[e.Source] = d
					next = append(next, e.Source)
				}
			}
		}
		frontier = next
	}
	frontier = []string{qn}
	for d := 1; d <= depth; d++ {
		var next []string
		for _, f := range frontier {
			for _, e := range downAdj[f] {
				if _, ok := nodes[e.Target]; !ok {
					addNode(e.Target, "down", d)
					downDepth[e.Target] = d
					next = append(next, e.Target)
				}
			}
		}
		frontier = next
	}

	keep := map[string]bool{qn: true}
	for id := range upDepth {
		keep[id] = true
	}
	for id := range downDepth {
		keep[id] = true
	}
	// 边 = 已探索集合内的全部边（与前端 collectEdges 口径一致，不限于 BFS 遍历路径）。
	edgeOut := collectEdges(edges, keep, crossPkgOf)
	nodeStats := buildNodeStats(edges, keep)
	for id, n := range nodes {
		if st, ok := nodeStats[id]; ok {
			n.FanIn, n.FanOut = st.FanIn, st.FanOut
		}
	}
	out := dto.FanPayload{
		Target: qn, Granularity: granularity, Edges: edgeOut,
		Stats:     fanStats(edges, qn, upDepth, downDepth, crossPkgOf, keep),
		NodeStats: nodeStats,
	}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, *n)
	}
	sort.Slice(out.Nodes, func(i, j int) bool {
		if out.Nodes[i].Side != out.Nodes[j].Side {
			return out.Nodes[i].Side < out.Nodes[j].Side
		}
		return out.Nodes[i].ID < out.Nodes[j].ID
	})
	out.RelationTypes, _ = s.repo.RelationTypes(dimID)
	return out, nil
}

// fanStats 计算蝴蝶图统计（对齐前端 fanGraph.FanStats 口径）。
func (s *Service) fanMethod(dimID int, qn string, depth int, types []string, hideImplDetail bool) (dto.FanPayload, error) {
	edges, err := s.repo.MethodEdges(dimID)
	if err != nil {
		return dto.FanPayload{}, err
	}
	if len(types) > 0 {
		filtered := edges[:0]
		for _, e := range edges {
			if intersectsAny(e.Types, types) {
				filtered = append(filtered, e)
			}
		}
		edges = filtered
	}

	pkgOf := s.repo.TypeToPackage(dimID)
	hostPkg := func(method string) string {
		return pkgOf[s.repo.HostTypeOf(method)]
	}
	crossPkgOf := func(a, b string) bool {
		pa, pb := hostPkg(a), hostPkg(b)
		return pa != "" && pb != "" && pa != pb
	}

	upAdj := map[string][]repository.Edge{}
	downAdj := map[string][]repository.Edge{}
	for _, e := range edges {
		downAdj[e.Source] = append(downAdj[e.Source], e)
		upAdj[e.Target] = append(upAdj[e.Target], e)
	}

	nodes := map[string]*dto.FanNode{}
	addNode := func(id, side string, d int) bool {
		if n, ok := nodes[id]; ok {
			return !n.IsImplDetail
		}
		isSugar, isImpl, _ := s.repo.ElementFlags(id)
		n := &dto.FanNode{
			ID: id, Label: lastSeg(id), Side: side, Depth: d, Kind: s.repo.KindOf(dimID, id),
			IsSugar: isSugar, IsImplDetail: isImpl, HostType: s.repo.HostTypeOf(id),
			CrossPkg: crossPkgOf(id, qn),
		}
		nodes[id] = n
		return !(hideImplDetail && isImpl)
	}
	addNode(qn, "center", 0)

	upDepth, downDepth := map[string]int{}, map[string]int{}
	frontier := []string{qn}
	for d := 1; d <= depth; d++ {
		var next []string
		for _, f := range frontier {
			for _, e := range upAdj[f] {
				if addNode(e.Source, "up", d) {
					upDepth[e.Source] = d
					next = append(next, e.Source)
				}
			}
			for _, e := range downAdj[f] {
				if addNode(e.Target, "down", d) {
					downDepth[e.Target] = d
					next = append(next, e.Target)
				}
			}
		}
		frontier = next
	}

	keep := map[string]bool{qn: true}
	for id := range upDepth {
		keep[id] = true
	}
	for id := range downDepth {
		keep[id] = true
	}
	edgeOut := collectEdges(edges, keep, crossPkgOf)
	nodeStats := buildNodeStats(edges, keep)
	for id, n := range nodes {
		if st, ok := nodeStats[id]; ok {
			n.FanIn, n.FanOut = st.FanIn, st.FanOut
		}
	}
	out := dto.FanPayload{
		Target: qn, Granularity: "method", Edges: edgeOut,
		Stats:     fanStats(edges, qn, upDepth, downDepth, crossPkgOf, keep),
		NodeStats: nodeStats,
	}
	for _, n := range nodes {
		if hideImplDetail && n.IsImplDetail {
			continue
		}
		out.Nodes = append(out.Nodes, *n)
	}
	sort.Slice(out.Nodes, func(i, j int) bool {
		if out.Nodes[i].Side != out.Nodes[j].Side {
			return out.Nodes[i].Side < out.Nodes[j].Side
		}
		return out.Nodes[i].ID < out.Nodes[j].ID
	})
	out.RelationTypes, _ = s.repo.RelationTypes(dimID)
	return out, nil
}

// ---- 映射辅助 ----
func fanStats(edges []repository.Edge, target string, upDepth, downDepth map[string]int,
	crossPkgOf func(a, b string) bool, keep map[string]bool) dto.FanStats {
	st := dto.FanStats{UpCount: len(upDepth), DownCount: len(downDepth)}
	for _, d := range upDepth {
		if d == 1 {
			st.DirectUp++
		}
	}
	for _, d := range downDepth {
		if d == 1 {
			st.DirectDown++
		}
	}
	for _, e := range edges {
		if e.Target == target {
			st.FanIn += e.Count
		}
		if e.Source == target {
			st.FanOut += e.Count
		}
		if keep[e.Source] && keep[e.Target] && crossPkgOf(e.Source, e.Target) {
			st.CrossPkg++
		}
	}
	return st
}

// buildNodeStats 生成每节点扇入扇出明细。
// 只对 keep 内的节点输出（避免把上万节点的全图明细塞进响应）；
func buildNodeStats(edges []repository.Edge, keep map[string]bool) map[string]dto.FanNodeStat {
	inAdj := map[string]map[string]*dto.FanNeighbor{}
	outAdj := map[string]map[string]*dto.FanNeighbor{}
	add := func(m map[string]map[string]*dto.FanNeighbor, from, to string, e repository.Edge) {
		byID, ok := m[from]
		if !ok {
			byID = map[string]*dto.FanNeighbor{}
			m[from] = byID
		}
		if hit, ok := byID[to]; ok {
			hit.Count += e.Count
			hit.Types = mergeStrings(hit.Types, e.Types)
			return
		}
		byID[to] = &dto.FanNeighbor{ID: to, Count: e.Count, Types: e.Types}
	}
	for _, e := range edges {
		add(outAdj, e.Source, e.Target, e)
		add(inAdj, e.Target, e.Source, e)
	}

	out := make(map[string]dto.FanNodeStat, len(keep))
	for id := range keep {
		st := dto.FanNodeStat{ID: id, InNeighbors: []dto.FanNeighbor{}, OutNeighbors: []dto.FanNeighbor{}}
		for _, n := range inAdj[id] {
			st.FanIn += n.Count
			st.InNeighbors = append(st.InNeighbors, *n)
		}
		for _, n := range outAdj[id] {
			st.FanOut += n.Count
			st.OutNeighbors = append(st.OutNeighbors, *n)
		}
		sortNeighbors(st.InNeighbors)
		sortNeighbors(st.OutNeighbors)
		out[id] = st
	}
	return out
}

// collectEdges 收集两端都在 keep 内的边（保持真实方向，按 source→target 去重）。
func collectEdges(edges []repository.Edge, keep map[string]bool, crossPkgOf func(a, b string) bool) []dto.FanEdge {
	var out []dto.FanEdge
	seen := map[string]bool{}
	for _, e := range edges {
		if !keep[e.Source] || !keep[e.Target] {
			continue
		}
		key := e.Source + "\x00" + e.Target
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, dto.FanEdge{
			Source: e.Source, Target: e.Target, Type: e.Type, Types: e.Types, Count: e.Count,
			CrossPkg: crossPkgOf(e.Source, e.Target),
		})
	}
	return out
}

// sortNeighbors 按引用次数降序、id 升序排列邻居。
func sortNeighbors(list []dto.FanNeighbor) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return list[i].ID < list[j].ID
	})
}

func firstOr(v []string) string {
	if len(v) > 0 {
		return v[0]
	}
	return ""
}

func mergeStrings(a, b []string) []string {
	set := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, v := range append(append([]string{}, a...), b...) {
		if !set[v] {
			set[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func intersectsAny(a, b []string) bool {
	set := map[string]bool{}
	for _, v := range b {
		set[v] = true
	}
	for _, v := range a {
		if set[v] {
			return true
		}
	}
	return false
}
