package qoder

import (
	"strings"
	"testing"
)

// ====== Test detectBaseIndentation ======

func TestDetectBaseIndentation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "正确格式：同级语句",
			// 使用字符串拼接避免额外缩进
			input: strings.Join([]string{
				"                logger.info(\"进行名称精确查询，先查询缓存\");",
				"                for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"                    if (entry.getValue().getName().equals(request.getName())) {",
				"                        List<Product> matchedProducts = new ArrayList<>();",
				"                    }",
				"                }",
			}, "\n"),
			expected: "                ", // 16个空格 - 第一行的缩进
		},
		{
			name: "错误格式：第二行缩进过多",
			// 使用字符串拼接避免额外缩进
			input: strings.Join([]string{
				"                logger.info(\"进行名称精确查询，先查询缓存\");",
				"                                for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"                                    if (entry.getValue().getName().equals(request.getName())) {",
				"                                        List<Product> matchedProducts = new ArrayList<>();",
				"                                    }",
				"                                }",
			}, "\n"),
			expected: "                ", // 16个空格 - 第一行的缩进（不再是最小的）
		},
		{
			name:     "制表符缩进",
			input:    "\tfunction test() {\n\t\treturn true;\n\t}",
			expected: "\t", // 第一行的缩进
		},
		{
			name:     "混合缩进",
			input:    "    function test() {" + "\n" + "\t\treturn true;" + "\n" + "    }",
			expected: "    ", // 第一行的缩进（4个空格）
		},
		{
			name:     "无缩进",
			input:    "function test() {\nreturn true;\n}",
			expected: "", // 第一行无缩进
		},
		{
			name:     "包含空行",
			input:    "    line1\n\n    line2\n        line3",
			expected: "    ", // 第一行的缩进
		},
		{
			name:     "只有空行",
			input:    "\n\n\n",
			expected: "", // 没有非空行
		},
		{
			name:     "第一行无缩进，后续行有缩进",
			input:    "logger.info();\n    for (int i = 0; i < 10; i++) {\n        System.out.println(i);\n    }",
			expected: "", // 第一行无缩进
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectBaseIndentation(tt.input)
			if result != tt.expected {
				t.Errorf("detectBaseIndentation() = %q, expected %q", result, tt.expected)
				t.Logf("Input lines:")
				lines := strings.Split(tt.input, "\n")
				for i, line := range lines {
					if strings.TrimSpace(line) != "" {
						indent := getIndentation(line)
						t.Logf("  Line %d: indent=%q (len=%d), content=%q", i+1, indent, len(indent), strings.TrimSpace(line))
					}
				}
			}
		})
	}
}

// ====== Test removeBaseIndentation ======

func TestRemoveBaseIndentation(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		baseIndent string
		expected   string
	}{
		{
			name: "移除4个空格基础缩进",
			input: `    line1
        line2
            line3`,
			baseIndent: "    ",
			expected: `line1
    line2
        line3`,
		},
		{
			name:       "移除制表符基础缩进",
			input:      "\tline1\n\t\tline2\n\t\t\tline3",
			baseIndent: "\t",
			expected:   "line1\n\tline2\n\t\tline3",
		},
		{
			name:       "处理空行",
			input:      "    line1\n\n    line2",
			baseIndent: "    ",
			expected:   "line1\n\nline2",
		},
		{
			name:       "基础缩进为空",
			input:      "line1\n    line2",
			baseIndent: "",
			expected:   "line1\n    line2",
		},
		{
			name: "真实例子：正确格式的suggestion",
			// 使用字符串拼接避免额外缩进
			input: strings.Join([]string{
				"                logger.info(\"进行名称精确查询，先查询缓存\");",
				"                for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"                    if (entry.getValue().getName().equals(request.getName())) {",
				"                        List<Product> matchedProducts = new ArrayList<>();",
				"                    }",
				"                }",
			}, "\n"),
			baseIndent: "                ", // 16个空格
			expected: strings.Join([]string{
				"logger.info(\"进行名称精确查询，先查询缓存\");",
				"for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"    if (entry.getValue().getName().equals(request.getName())) {",
				"        List<Product> matchedProducts = new ArrayList<>();",
				"    }",
				"}",
			}, "\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeBaseIndentation(tt.input, tt.baseIndent)
			if result != tt.expected {
				t.Errorf("removeBaseIndentation() result mismatch")
				t.Logf("Expected:\n%q", tt.expected)
				t.Logf("Got:\n%q", result)

				// 逐行比较
				expectedLines := strings.Split(tt.expected, "\n")
				actualLines := strings.Split(result, "\n")
				maxLen := len(expectedLines)
				if len(actualLines) > maxLen {
					maxLen = len(actualLines)
				}

				for i := 0; i < maxLen; i++ {
					expectedLine := ""
					actualLine := ""
					if i < len(expectedLines) {
						expectedLine = expectedLines[i]
					}
					if i < len(actualLines) {
						actualLine = actualLines[i]
					}
					if expectedLine != actualLine {
						t.Errorf("Line %d mismatch:\n  Expected: %q\n  Got:      %q", i+1, expectedLine, actualLine)
					}
				}
			}
		})
	}
}

// ====== Test applyBaseIndentation ======

func TestApplyBaseIndentation(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		targetIndent string
		expected     string
	}{
		{
			name: "添加制表符缩进",
			input: `line1
    line2
        line3`,
			targetIndent: "\t",
			expected:     "\tline1\n\t    line2\n\t        line3",
		},
		{
			name:         "添加空格缩进",
			input:        "line1\n\tline2\n\t\tline3",
			targetIndent: "    ",
			expected:     "    line1\n    \tline2\n    \t\tline3",
		},
		{
			name:         "处理空行",
			input:        "line1\n\nline2",
			targetIndent: "\t",
			expected:     "\tline1\n\n\tline2",
		},
		{
			name:         "目标缩进为空",
			input:        "line1\n    line2",
			targetIndent: "",
			expected:     "line1\n    line2",
		},
		{
			name: "真实例子：应用制表符缩进",
			// 使用字符串拼接避免额外缩进
			input: strings.Join([]string{
				"logger.info(\"进行名称精确查询，先查询缓存\");",
				"for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"    if (entry.getValue().getName().equals(request.getName())) {",
				"        List<Product> matchedProducts = new ArrayList<>();",
				"    }",
				"}",
			}, "\n"),
			targetIndent: "\t\t\t\t", // 4个制表符
			expected: strings.Join([]string{
				"\t\t\t\tlogger.info(\"进行名称精确查询，先查询缓存\");",
				"\t\t\t\tfor (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"\t\t\t\t    if (entry.getValue().getName().equals(request.getName())) {",
				"\t\t\t\t        List<Product> matchedProducts = new ArrayList<>();",
				"\t\t\t\t    }",
				"\t\t\t\t}",
			}, "\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyBaseIndentation(tt.input, tt.targetIndent)
			if result != tt.expected {
				t.Errorf("applyBaseIndentation() result mismatch")
				t.Logf("Expected:\n%q", tt.expected)
				t.Logf("Got:\n%q", result)

				// 逐行比较
				expectedLines := strings.Split(tt.expected, "\n")
				actualLines := strings.Split(result, "\n")
				maxLen := len(expectedLines)
				if len(actualLines) > maxLen {
					maxLen = len(actualLines)
				}

				for i := 0; i < maxLen; i++ {
					expectedLine := ""
					actualLine := ""
					if i < len(expectedLines) {
						expectedLine = expectedLines[i]
					}
					if i < len(actualLines) {
						actualLine = actualLines[i]
					}
					if expectedLine != actualLine {
						t.Errorf("Line %d mismatch:\n  Expected: %q\n  Got:      %q", i+1, expectedLine, actualLine)
					}
				}
			}
		})
	}
}

// ====== Test getIndentation ======

func TestGetIndentation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "无缩进",
			input:    "hello world",
			expected: "",
		},
		{
			name:     "4个空格",
			input:    "    hello world",
			expected: "    ",
		},
		{
			name:     "1个制表符",
			input:    "\thello world",
			expected: "\t",
		},
		{
			name:     "混合缩进",
			input:    "  \t  hello world",
			expected: "  \t  ",
		},
		{
			name:     "空行",
			input:    "",
			expected: "",
		},
		{
			name:     "只有空格",
			input:    "    ",
			expected: "    ",
		},
		{
			name:     "16个空格",
			input:    "                hello",
			expected: "                ",
		},
		{
			name:     "32个空格",
			input:    "                                hello",
			expected: "                                ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIndentation(tt.input)
			if result != tt.expected {
				t.Errorf("getIndentation(%q) = %q, expected %q", tt.input, result, tt.expected)
				t.Logf("Result length: %d, Expected length: %d", len(result), len(tt.expected))
			}
		})
	}
}

// ====== Integration Tests ======

func TestFullIndentationWorkflow(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		targetIndent  string
		expectedFinal string
	}{
		{
			name: "完整流程：正确格式的suggestion",
			// 使用字符串拼接避免额外缩进
			input: strings.Join([]string{
				"                logger.info(\"进行名称精确查询，先查询缓存\");",
				"                for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"                    if (entry.getValue().getName().equals(request.getName())) {",
				"                        List<Product> matchedProducts = new ArrayList<>();",
				"                        matchedProducts.add(entry.getValue());",
				"                        return response;",
				"                    }",
				"                }",
			}, "\n"),
			targetIndent: "\t\t\t\t",
			expectedFinal: strings.Join([]string{
				"\t\t\t\tlogger.info(\"进行名称精确查询，先查询缓存\");",
				"\t\t\t\tfor (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"\t\t\t\t    if (entry.getValue().getName().equals(request.getName())) {",
				"\t\t\t\t        List<Product> matchedProducts = new ArrayList<>();",
				"\t\t\t\t        matchedProducts.add(entry.getValue());",
				"\t\t\t\t        return response;",
				"\t\t\t\t    }",
				"\t\t\t\t}",
			}, "\n"),
		},
		{
			name: "完整流程：正确格式的suggestion（较复杂的例子）",
			// 使用字符串拼接避免额外缩进
			input: strings.Join([]string{
				"                logger.info(\"进行名称精确查询，先查询缓存\");",
				"                for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"                    if (entry.getValue().getName().equals(request.getName())) {",
				"                        List<Product> matchedProducts = new ArrayList<>();",
				"                        matchedProducts.add(entry.getValue());",
				"                        response.setRecords(matchedProducts);",
				"                        return response;",
				"                    }",
				"                }",
			}, "\n"),
			targetIndent: "\t\t\t\t",
			expectedFinal: strings.Join([]string{
				"\t\t\t\tlogger.info(\"进行名称精确查询，先查询缓存\");",
				"\t\t\t\tfor (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"\t\t\t\t    if (entry.getValue().getName().equals(request.getName())) {",
				"\t\t\t\t        List<Product> matchedProducts = new ArrayList<>();",
				"\t\t\t\t        matchedProducts.add(entry.getValue());",
				"\t\t\t\t        response.setRecords(matchedProducts);",
				"\t\t\t\t        return response;",
				"\t\t\t\t    }",
				"\t\t\t\t}",
			}, "\n"),
		},
		{
			name: "完整流程：第一行无缩进的情况",
			// 使用字符串拼接避免额外缩进
			input: strings.Join([]string{
				"logger.info(\"进行名称精确查询，先查询缓存\");",
				"    for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"        if (entry.getValue().getName().equals(request.getName())) {",
				"            return response;",
				"        }",
				"    }",
			}, "\n"),
			targetIndent: "\t\t",
			expectedFinal: strings.Join([]string{
				"\t\tlogger.info(\"进行名称精确查询，先查询缓存\");",
				"\t\t    for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"\t\t        if (entry.getValue().getName().equals(request.getName())) {",
				"\t\t            return response;",
				"\t\t        }",
				"\t\t    }",
			}, "\n"),
		},
		{
			name: "完整流程：错误格式的suggestion（不规范缩进）",
			// 使用字符串拼接避免额外缩进
			input: strings.Join([]string{
				"    logger.info(\"进行名称精确查询，先查询缓存\");",
				"      for (Map.Entry<Long, Product> entry : productCache.entrySet()) {", // 第二行缩进不一致（6个空格）
				"          if (entry.getValue().getName().equals(request.getName())) {",  // 第三行缩进不一致（10个空格）
				"        return response;", // 第四行缩进不一致（8个空格）
				"      }",
				"    }",
			}, "\n"),
			targetIndent: "\t\t",
			expectedFinal: strings.Join([]string{
				"\t\tlogger.info(\"进行名称精确查询，先查询缓存\");",                                   // 第一行的缩进被移除，应用新的目标缩进
				"\t\t  for (Map.Entry<Long, Product> entry : productCache.entrySet()) {", // 保持相对缩进结构
				"\t\t      if (entry.getValue().getName().equals(request.getName())) {",
				"\t\t    return response;",
				"\t\t  }",
				"\t\t}",
			}, "\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 执行完整的缩进调整流程
			baseIndent := detectBaseIndentation(tt.input)
			t.Logf("Detected base indentation: %q (len=%d)", baseIndent, len(baseIndent))

			unindented := removeBaseIndentation(tt.input, baseIndent)
			t.Logf("After removing base indentation:")
			unindentedLines := strings.Split(unindented, "\n")
			for i, line := range unindentedLines {
				if strings.TrimSpace(line) != "" {
					indent := getIndentation(line)
					t.Logf("  Line %d: indent=%q (len=%d), content=%q", i+1, indent, len(indent), strings.TrimSpace(line))
				}
			}

			final := applyBaseIndentation(unindented, tt.targetIndent)

			if final != tt.expectedFinal {
				t.Errorf("Full workflow result mismatch")
				t.Logf("Expected:\n%q", tt.expectedFinal)
				t.Logf("Got:\n%q", final)

				// 详细分析差异
				expectedLines := strings.Split(tt.expectedFinal, "\n")
				actualLines := strings.Split(final, "\n")
				for i := 0; i < len(expectedLines) && i < len(actualLines); i++ {
					if expectedLines[i] != actualLines[i] {
						t.Errorf("Line %d differs:\n  Expected: %q\n  Got:      %q", i+1, expectedLines[i], actualLines[i])
					}
				}
			}
		})
	}
}

// ====== End-to-End Tests ======

func TestEndToEndIndentationAdjustment(t *testing.T) {
	tests := []struct {
		name             string
		originalCode     string // 模拟GitHub文件中第line行的原始代码
		suggestionCode   string // 模拟LLM生成的suggestion代码
		expectedAdjusted string // 期望调整后的suggestion代码
		description      string // 测试场景描述
	}{
		{
			name: "E2E测试：空格缩进文件中的suggestion调整",
			// 模拟目标文件使用4个空格缩进的情况
			originalCode: "        if (condition) { // 第89行，8个空格缩进",
			// 模拟LLM生成的suggestion，使用了16个空格的基础缩进
			suggestionCode: strings.Join([]string{
				"                logger.info(\"进行名称精确查询，先查询缓存\");",
				"                for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"                    if (entry.getValue().getName().equals(request.getName())) {",
				"                        List<Product> matchedProducts = new ArrayList<>();",
				"                        return matchedProducts;",
				"                    }",
				"                }",
			}, "\n"),
			// 期望调整后应该使用8个空格的基础缩进
			expectedAdjusted: strings.Join([]string{
				"        logger.info(\"进行名称精确查询，先查询缓存\");",
				"        for (Map.Entry<Long, Product> entry : productCache.entrySet()) {",
				"            if (entry.getValue().getName().equals(request.getName())) {",
				"                List<Product> matchedProducts = new ArrayList<>();",
				"                return matchedProducts;",
				"            }",
				"        }",
			}, "\n"),
			description: "模拟在Java文件第89行位置（8个空格缩进）应用suggestion，LLM生成的suggestion有16个空格基础缩进，需要调整为8个空格基础缩进",
		},
		{
			name: "E2E测试：制表符缩进文件中的suggestion调整",
			// 模拟目标文件使用制表符缩进的情况
			originalCode: "\t\t\tpublic void processRequest() { // 第45行，3个制表符缩进",
			// 模拟LLM生成的suggestion，使用了空格缩进
			suggestionCode: strings.Join([]string{
				"        // 添加缓存检查逻辑",
				"        if (cache.containsKey(key)) {",
				"            return cache.get(key);",
				"        }",
				"        // 执行实际查询",
				"        Result result = performQuery(key);",
				"        cache.put(key, result);",
				"        return result;",
			}, "\n"),
			// 期望调整后应该使用3个制表符的基础缩进
			expectedAdjusted: strings.Join([]string{
				"\t\t\t// 添加缓存检查逻辑",
				"\t\t\tif (cache.containsKey(key)) {",
				"\t\t\t    return cache.get(key);",
				"\t\t\t}",
				"\t\t\t// 执行实际查询",
				"\t\t\tResult result = performQuery(key);",
				"\t\t\tcache.put(key, result);",
				"\t\t\treturn result;",
			}, "\n"),
			description: "模拟在Java文件第45行位置（3个制表符缩进）应用suggestion，LLM生成的suggestion使用8个空格基础缩进，需要调整为3个制表符基础缩进",
		},
		{
			name: "E2E测试：suggestion代码无基础缩进的情况",
			// 模拟目标文件有缩进的情况
			originalCode: "            // 在方法内部添加代码 // 第156行，12个空格缩进",
			// 模拟LLM生成的suggestion，没有基础缩进
			suggestionCode: strings.Join([]string{
				"// 验证输入参数",
				"if (request == null || request.getName() == null) {",
				"    throw new IllegalArgumentException(\"Request or name cannot be null\");",
				"}",
				"// 调用服务层方法",
				"return productService.findByName(request.getName());",
			}, "\n"),
			// 期望调整后应该使用12个空格的基础缩进
			expectedAdjusted: strings.Join([]string{
				"            // 验证输入参数",
				"            if (request == null || request.getName() == null) {",
				"                throw new IllegalArgumentException(\"Request or name cannot be null\");",
				"            }",
				"            // 调用服务层方法",
				"            return productService.findByName(request.getName());",
			}, "\n"),
			description: "模拟在Java文件第156行位置（12个空格缩进）应用suggestion，LLM生成的suggestion没有基础缩进，需要添加12个空格基础缩进",
		},
		{
			name: "E2E测试：复杂嵌套结构的suggestion调整",
			// 模拟目标文件使用制表符缩进
			originalCode: "\t\tfor (Product product : products) { // 第78行，2个制表符缩进",
			// 模拟LLM生成的复杂嵌套suggestion
			suggestionCode: strings.Join([]string{
				"    // 检查产品是否有效",
				"    if (product != null && product.isActive()) {",
				"        // 检查库存",
				"        if (product.getStock() > 0) {",
				"            // 应用折扣",
				"            if (product.hasDiscount()) {",
				"                double discountedPrice = product.getPrice() * (1 - product.getDiscountRate());",
				"                product.setDiscountedPrice(discountedPrice);",
				"            }",
				"            // 添加到结果列表",
				"            validProducts.add(product);",
				"        } else {",
				"            logger.warn(\"Product {} is out of stock\", product.getId());",
				"        }",
				"    }",
			}, "\n"),
			// 期望调整后应该使用2个制表符的基础缩进
			expectedAdjusted: strings.Join([]string{
				"\t\t// 检查产品是否有效",
				"\t\tif (product != null && product.isActive()) {",
				"\t\t    // 检查库存",
				"\t\t    if (product.getStock() > 0) {",
				"\t\t        // 应用折扣",
				"\t\t        if (product.hasDiscount()) {",
				"\t\t            double discountedPrice = product.getPrice() * (1 - product.getDiscountRate());",
				"\t\t            product.setDiscountedPrice(discountedPrice);",
				"\t\t        }",
				"\t\t        // 添加到结果列表",
				"\t\t        validProducts.add(product);",
				"\t\t    } else {",
				"\t\t        logger.warn(\"Product {} is out of stock\", product.getId());",
				"\t\t    }",
				"\t\t}",
			}, "\n"),
			description: "模拟在Java文件第78行位置（2个制表符缩进）应用复杂嵌套suggestion，LLM生成的suggestion有4个空格基础缩进，需要调整为2个制表符基础缩进，并保持内部嵌套结构",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("\n=== %s ===", tt.description)

			// 步骤1：从原始代码中提取目标缩进
			targetIndentation := getIndentation(tt.originalCode)
			t.Logf("步骤1 - 从原始代码提取目标缩进: %q (长度=%d)", targetIndentation, len(targetIndentation))

			// 步骤2：检测suggestion的基础缩进
			baseIndentation := detectBaseIndentation(tt.suggestionCode)
			t.Logf("步骤2 - 检测suggestion基础缩进: %q (长度=%d)", baseIndentation, len(baseIndentation))

			// 步骤3：移除suggestion的基础缩进
			unindentedSuggestion := removeBaseIndentation(tt.suggestionCode, baseIndentation)
			t.Logf("步骤3 - 移除基础缩进后的suggestion:")
			unindentedLines := strings.Split(unindentedSuggestion, "\n")
			for i, line := range unindentedLines {
				if strings.TrimSpace(line) != "" {
					indent := getIndentation(line)
					t.Logf("    第%d行: 缩进=%q (长度=%d), 内容=%q", i+1, indent, len(indent), strings.TrimSpace(line))
				}
			}

			// 步骤4：应用目标缩进
			finalSuggestion := applyBaseIndentation(unindentedSuggestion, targetIndentation)
			t.Logf("步骤4 - 应用目标缩进后的最终结果:")
			finalLines := strings.Split(finalSuggestion, "\n")
			for i, line := range finalLines {
				if strings.TrimSpace(line) != "" {
					indent := getIndentation(line)
					t.Logf("    第%d行: 缩进=%q (长度=%d), 内容=%q", i+1, indent, len(indent), strings.TrimSpace(line))
				}
			}

			// 验证最终结果
			if finalSuggestion != tt.expectedAdjusted {
				t.Errorf("❌ 端到端测试失败！")
				t.Logf("期望结果:\n%q", tt.expectedAdjusted)
				t.Logf("实际结果:\n%q", finalSuggestion)

				// 逐行比较差异
				expectedLines := strings.Split(tt.expectedAdjusted, "\n")
				actualLines := strings.Split(finalSuggestion, "\n")
				maxLen := len(expectedLines)
				if len(actualLines) > maxLen {
					maxLen = len(actualLines)
				}

				for i := 0; i < maxLen; i++ {
					expectedLine := ""
					actualLine := ""
					if i < len(expectedLines) {
						expectedLine = expectedLines[i]
					}
					if i < len(actualLines) {
						actualLine = actualLines[i]
					}
					if expectedLine != actualLine {
						t.Errorf("第%d行不匹配:\n  期望: %q\n  实际: %q", i+1, expectedLine, actualLine)
					}
				}
			} else {
				t.Logf("✅ 端到端测试成功！缩进调整正确完成")
			}
		})
	}
}
