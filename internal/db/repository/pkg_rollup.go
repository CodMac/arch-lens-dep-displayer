package repository

import (
	"sort"
)

// BuildPkgRollup 初始化包级上卷行（P9）：直接类型数写入，其余列置 0。
func (r *Repository) BuildPkgRollup(dimID, nsLevel, typeLevel int) error {
	q := `INSERT INTO pkg_rollup (dimension_id, qn, subtree_type_count, subtree_cycle_groups,
			subtree_high_fanin, subtree_high_fanout, direct_type_count)
		SELECT ?, h.qn, 0, 0, 0, 0,
		       (SELECT COUNT(*) FROM hierarchy_node c
		        WHERE c.dimension_id = h.dimension_id AND c.level = ? AND c.parent_qn = h.qn)
		FROM hierarchy_node h WHERE h.dimension_id = ? AND h.level = ?`
	if _, err := r.db().Exec(q, dimID, typeLevel, dimID, nsLevel); err != nil {
		return err
	}
	_, err := r.db().Exec(`UPDATE pkg_rollup SET subtree_type_count = direct_type_count
		WHERE dimension_id = ?`, dimID)
	return err
}

// RollupColumn 按 depth 降序把子包的 src 列累加到父包。
func (r *Repository) RollupColumn(dimID, nsLevel int, dst, src string) error {
	list, err := r.ParentPairsByDepthDesc(dimID, nsLevel)
	if err != nil {
		return err
	}
	q := `UPDATE pkg_rollup p SET ` + dst + ` = ` + dst + ` + COALESCE(
		(SELECT c.` + dst + ` FROM pkg_rollup c WHERE c.dimension_id = p.dimension_id AND c.qn = ?), 0)
		WHERE p.dimension_id = ? AND p.qn = ?`
	for _, t := range list {
		if t.Parent == "" {
			continue
		}
		if _, err := r.db().Exec(q, t.QN, dimID, t.Parent); err != nil {
			return err
		}
	}
	return nil
}

// IncrementRollup 把某列的计数 +1（违规/环组向祖先链累加）。
func (r *Repository) IncrementRollup(dimID int, column, qn string) error {
	q := `UPDATE pkg_rollup SET ` + column + ` = ` + column + ` + 1 WHERE dimension_id = ? AND qn = ?`
	_, err := r.db().Exec(q, dimID, qn)
	return err
}

// ClearPkgRollup 清空某维度的包级上卷。
func (r *Repository) ClearPkgRollup(dimID int) error {
	_, err := r.db().Exec(`DELETE FROM pkg_rollup WHERE dimension_id = ?`, dimID)
	return err
}

func (r *Repository) SubtreeTypeCount(dimID int, qn string) int {
	return r.pkgTree(dimID).typeCount[qn]
}

func (r *Repository) pkgTree(dimID int) *pkgTreeData {
	t := &pkgTreeData{children: map[string][]string{}, typeCount: map[string]int{}, depth: map[string]int{}}
	nsLevel := r.LevelOfRole(dimID, "namespace")

	pkgRows, err := r.db().Query(`SELECT qn, COALESCE(parent_qn,''), COALESCE(depth,0)
		FROM hierarchy_node WHERE dimension_id = ? AND level = ?`, dimID, nsLevel)
	if err != nil {
		return t
	}
	for pkgRows.Next() {
		var qn, parent string
		var depth int
		if err := pkgRows.Scan(&qn, &parent, &depth); err != nil {
			continue
		}
		t.children[parent] = append(t.children[parent], qn)
		t.depth[qn] = depth
	}
	pkgRows.Close()

	// 预计算层已按 parent_qn 自底向上累加，直接读即可（避免每次请求重扫 hierarchy_node）。
	rollRows, err := r.db().Query(`SELECT qn, subtree_type_count FROM pkg_rollup WHERE dimension_id = ?`, dimID)
	if err != nil {
		return t
	}
	rolled := 0
	for rollRows.Next() {
		var qn string
		var n int
		if err := rollRows.Scan(&qn, &n); err != nil {
			continue
		}
		t.typeCount[qn] = n
		rolled++
	}
	rollRows.Close()
	if rolled > 0 {
		return t
	}

	// 回退：pkg_rollup 未生成（如仅跑了 hierarchy 未跑 preprocess）时实时累加。
	typeLevel := r.LevelOfRole(dimID, "type")
	typeRows, err := r.db().Query(`SELECT COALESCE(parent_qn,''), COUNT(*) FROM hierarchy_node
		WHERE dimension_id = ? AND level = ? GROUP BY 1`, dimID, typeLevel)
	if err != nil {
		return t
	}
	for typeRows.Next() {
		var p string
		var c int
		if err := typeRows.Scan(&p, &c); err != nil {
			continue
		}
		t.typeCount[p] += c
	}
	typeRows.Close()

	t.computeSizes()
	return t
}

func (t *pkgTreeData) computeSizes() {
	pkgs := make([]string, 0, len(t.depth))
	for qn := range t.depth {
		pkgs = append(pkgs, qn)
	}
	sort.Slice(pkgs, func(i, j int) bool { return t.depth[pkgs[i]] > t.depth[pkgs[j]] })
	parentOf := map[string]string{}
	for parent, kids := range t.children {
		for _, k := range kids {
			parentOf[k] = parent
		}
	}
	for _, p := range pkgs {
		if par := parentOf[p]; par != "" {
			t.typeCount[par] += t.typeCount[p]
		}
	}
}

type pkgTreeData struct {
	children  map[string][]string
	typeCount map[string]int
	depth     map[string]int
}
