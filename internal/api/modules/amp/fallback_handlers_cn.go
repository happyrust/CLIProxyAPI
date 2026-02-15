// Package amp 提供AMP（Anthropic Model Provider）模块的处理逻辑
// 主要功能：模型请求的路由、回退处理和模型映射
package amp

import (
	"bytes"
	"io"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// AmpRouteType 表示为AMP请求做出的路由决策类型
type AmpRouteType string

const (
	// RouteTypeLocalProvider 表示请求由本地OAuth提供商处理（免费）
	RouteTypeLocalProvider AmpRouteType = "LOCAL_PROVIDER"
	// RouteTypeModelMapping 表示请求被重新映射到另一个可用模型（免费）
	RouteTypeModelMapping AmpRouteType = "MODEL_MAPPING"
	// RouteTypeAmpCredits 表示请求被转发到ampcode.com（使用AMP积分）
	RouteTypeAmpCredits AmpRouteType = "AMP_CREDITS"
	// RouteTypeNoProvider 表示没有可用的提供商或回退方案
	RouteTypeNoProvider AmpRouteType = "NO_PROVIDER"
)

// MappedModelContextKey 是Gin上下文中用于传递映射模型名称的键
const MappedModelContextKey = "mapped_model"

// logAmpRouting 使用结构化字段记录AMP请求的路由决策
// 参数：
//   - routeType: 路由类型（本地提供商、模型映射、AMP积分或无提供商）
//   - requestedModel: 用户请求的原始模型名称
//   - resolvedModel: 解析后的模型名称（可能与原始名称不同）
//   - provider: 提供商名称
//   - path: 请求路径
func logAmpRouting(routeType AmpRouteType, requestedModel, resolvedModel, provider, path string) {
	// 初始化日志字段
	fields := log.Fields{
		"component":       "amp-routing",
		"route_type":      string(routeType),
		"requested_model": requestedModel,
		"path":            path,
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	// 如果解析后的模型与原始模型不同，记录解析后的模型
	if resolvedModel != "" && resolvedModel != requestedModel {
		fields["resolved_model"] = resolvedModel
	}
	// 如果提供商不为空，记录提供商信息
	if provider != "" {
		fields["provider"] = provider
	}

	// 根据路由类型记录不同的日志信息
	switch routeType {
	case RouteTypeLocalProvider:
		// 使用本地提供商：免费，来源是本地OAuth
		fields["cost"] = "free"
		fields["source"] = "local_oauth"
		log.WithFields(fields).Debugf("amp using local provider for model: %s", requestedModel)

	case RouteTypeModelMapping:
		// 使用模型映射：免费，来源是本地OAuth
		fields["cost"] = "free"
		fields["source"] = "local_oauth"
		fields["mapping"] = requestedModel + " -> " + resolvedModel
		// 模型映射已在mapper中记录过，这里避免重复记录

	case RouteTypeAmpCredits:
		// 转发到ampcode.com：使用AMP积分
		fields["cost"] = "amp_credits"
		fields["source"] = "ampcode.com"
		fields["model_id"] = requestedModel // 显式记录model_id便于配置参考
		log.WithFields(fields).Warnf("forwarding to ampcode.com (uses amp credits) - model_id: %s | To use local provider, add to config: ampcode.model-mappings: [{from: \"%s\", to: \"<your-local-model>\"}]", requestedModel, requestedModel)

	case RouteTypeNoProvider:
		// 没有可用的提供商：无成本，来源是错误
		fields["cost"] = "none"
		fields["source"] = "error"
		fields["model_id"] = requestedModel // 显式记录model_id便于配置参考
		log.WithFields(fields).Warnf("no provider available for model_id: %s", requestedModel)
	}
}

// FallbackHandler 用回退逻辑包装标准处理器
// 当模型的提供商在CLIProxyAPI中不可用时，将请求转发到ampcode.com
type FallbackHandler struct {
	// getProxy 返回反向代理实例的函数（支持延迟初始化）
	getProxy func() *httputil.ReverseProxy
	// modelMapper 模型映射器，用于将一个模型映射到另一个模型
	modelMapper ModelMapper
	// forceModelMappings 返回是否强制使用模型映射的函数
	forceModelMappings func() bool
}

// NewFallbackHandler 创建一个新的回退处理器包装器
// 参数：
//   - getProxy: 获取反向代理的函数，支持延迟初始化（在路由创建后创建代理时很有用）
// 返回：新的FallbackHandler实例
func NewFallbackHandler(getProxy func() *httputil.ReverseProxy) *FallbackHandler {
	return &FallbackHandler{
		getProxy:           getProxy,
		forceModelMappings: func() bool { return false },
	}
}

// NewFallbackHandlerWithMapper 创建一个支持模型映射的新回退处理器
// 参数：
//   - getProxy: 获取反向代理的函数
//   - mapper: 模型映射器实例
//   - forceModelMappings: 是否强制使用模型映射的函数
// 返回：新的FallbackHandler实例
func NewFallbackHandlerWithMapper(getProxy func() *httputil.ReverseProxy, mapper ModelMapper, forceModelMappings func() bool) *FallbackHandler {
	// 如果forceModelMappings为nil，设置默认值为false
	if forceModelMappings == nil {
		forceModelMappings = func() bool { return false }
	}
	return &FallbackHandler{
		getProxy:           getProxy,
		modelMapper:        mapper,
		forceModelMappings: forceModelMappings,
	}
}

// SetModelMapper 为此处理器设置模型映射器（支持后期绑定）
// 参数：
//   - mapper: 模型映射器实例
func (fh *FallbackHandler) SetModelMapper(mapper ModelMapper) {
	fh.modelMapper = mapper
}

// WrapHandler 用回退逻辑包装gin.HandlerFunc
// 如果模型的提供商在CLIProxyAPI中未配置，则转发到ampcode.com
// 参数：
//   - handler: 原始的Gin处理器函数
// 返回：包装后的处理器函数
func (fh *FallbackHandler) WrapHandler(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取请求路径
		requestPath := c.Request.URL.Path

		// 读取请求体以提取模型名称
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Errorf("amp fallback: failed to read request body: %v", err)
			handler(c)
			return
		}

		// 恢复请求体供处理器读取
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// 尝试从请求体或URL路径中提取模型名称（Gemini使用URL路径）
		modelName := extractModelFromRequest(bodyBytes, c)
		if modelName == "" {
			// 无法确定模型，继续使用正常处理器
			handler(c)
			return
		}

		// 规范化模型名称（处理动态思考后缀，如"(xhigh)"）
		suffixResult := thinking.ParseSuffix(modelName)
		normalizedModel := suffixResult.ModelName
		thinkingSuffix := ""
		if suffixResult.HasSuffix {
			thinkingSuffix = "(" + suffixResult.RawSuffix + ")"
		}

		// resolveMappedModel 是一个内部函数，用于解析模型映射
		// 返回：映射后的模型名称和对应的提供商列表
		resolveMappedModel := func() (string, []string) {
			// 如果没有模型映射器，返回空
			if fh.modelMapper == nil {
				return "", nil
			}

			// 首先尝试使用原始模型名称进行映射
			mappedModel := fh.modelMapper.MapModel(modelName)
			// 如果没有映射，尝试使用规范化后的模型名称
			if mappedModel == "" {
				mappedModel = fh.modelMapper.MapModel(normalizedModel)
			}
			// 去除空格
			mappedModel = strings.TrimSpace(mappedModel)
			if mappedModel == "" {
				return "", nil
			}

			// 保留动态思考后缀（例如"(xhigh)"）当映射应用时
			// 除非目标模型已经指定了自己的思考后缀
			if thinkingSuffix != "" {
				mappedSuffixResult := thinking.ParseSuffix(mappedModel)
				if !mappedSuffixResult.HasSuffix {
					mappedModel += thinkingSuffix
				}
			}

			// 获取映射后模型的基础名称和对应的提供商
			mappedBaseModel := thinking.ParseSuffix(mappedModel).ModelName
			mappedProviders := util.GetProviderName(mappedBaseModel)
			if len(mappedProviders) == 0 {
				return "", nil
			}

			return mappedModel, mappedProviders
		}

		// 跟踪解析后的模型名称用于日志记录（如果应用了映射，可能会改变）
		resolvedModel := normalizedModel
		usedMapping := false
		var providers []string

		// 检查是否应该强制使用模型映射（优先于本地API密钥）
		forceMappings := fh.forceModelMappings != nil && fh.forceModelMappings()

		if forceMappings {
			// 强制模式：首先检查模型映射（优先于本地API密钥）
			// 这允许用户将Amp请求路由到他们首选的OAuth提供商
			if mappedModel, mappedProviders := resolveMappedModel(); mappedModel != "" {
				// 找到映射且提供商可用 - 重写请求体中的模型
				bodyBytes = rewriteModelInRequest(bodyBytes, mappedModel)
				c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				// 在上下文中存储映射后的模型（供检查它的处理器使用，如gemini bridge）
				c.Set(MappedModelContextKey, mappedModel)
				resolvedModel = mappedModel
				usedMapping = true
				providers = mappedProviders
			}

			// 如果没有应用映射，检查本地提供商
			if !usedMapping {
				providers = util.GetProviderName(normalizedModel)
			}
		} else {
			// 默认模式：首先检查本地提供商，然后检查映射作为回退
			providers = util.GetProviderName(normalizedModel)

			if len(providers) == 0 {
				// 没有配置提供商 - 检查是否有模型映射
				if mappedModel, mappedProviders := resolveMappedModel(); mappedModel != "" {
					// 找到映射且提供商可用 - 重写请求体中的模型
					bodyBytes = rewriteModelInRequest(bodyBytes, mappedModel)
					c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
					// 在上下文中存储映射后的模型（供检查它的处理器使用，如gemini bridge）
					c.Set(MappedModelContextKey, mappedModel)
					resolvedModel = mappedModel
					usedMapping = true
					providers = mappedProviders
				}
			}
		}

		// 如果没有可用的提供商，回退到ampcode.com
		if len(providers) == 0 {
			proxy := fh.getProxy()
			if proxy != nil {
				// 记录：转发到ampcode.com（使用AMP积分）
				logAmpRouting(RouteTypeAmpCredits, modelName, "", "", requestPath)

				// 再次恢复请求体供代理使用
				c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

				// 转发到ampcode.com
				proxy.ServeHTTP(c.Writer, c.Request)
				return
			}

			// 没有可用的代理，让正常处理器返回错误
			logAmpRouting(RouteTypeNoProvider, modelName, "", "", requestPath)
		}

		// 记录路由决策
		providerName := ""
		if len(providers) > 0 {
			providerName = providers[0]
		}

		if usedMapping {
			// 记录：模型被映射到另一个模型
			log.Debugf("amp model mapping: request %s -> %s", normalizedModel, resolvedModel)
			logAmpRouting(RouteTypeModelMapping, modelName, resolvedModel, providerName, requestPath)
			// 创建响应重写器，用于将响应中的模型名称改回原始名称
			rewriter := NewResponseRewriter(c.Writer, modelName)
			c.Writer = rewriter
			// 仅对本地处理路径过滤Anthropic-Beta头部
			filterAntropicBetaHeader(c)
			c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			handler(c)
			rewriter.Flush()
			log.Debugf("amp model mapping: response %s -> %s", resolvedModel, modelName)
		} else if len(providers) > 0 {
			// 记录：使用本地提供商（免费）
			logAmpRouting(RouteTypeLocalProvider, modelName, resolvedModel, providerName, requestPath)
			// 仅对本地处理路径过滤Anthropic-Beta头部
			filterAntropicBetaHeader(c)
			c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			handler(c)
		} else {
			// 没有提供商、没有映射、没有代理：回退到包装的处理器以返回错误响应
			c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			handler(c)
		}
	}
}

// filterAntropicBetaHeader 过滤Anthropic-Beta头部以移除需要特殊订阅的功能
// 当使用本地提供商时需要此操作（绕过Amp代理）
// 参数：
//   - c: Gin上下文
func filterAntropicBetaHeader(c *gin.Context) {
	// 获取Anthropic-Beta头部
	if betaHeader := c.Request.Header.Get("Anthropic-Beta"); betaHeader != "" {
		// 过滤掉需要特殊订阅的功能（如"context-1m-2025-08-07"）
		if filtered := filterBetaFeatures(betaHeader, "context-1m-2025-08-07"); filtered != "" {
			c.Request.Header.Set("Anthropic-Beta", filtered)
		} else {
			// 如果过滤后为空，删除该头部
			c.Request.Header.Del("Anthropic-Beta")
		}
	}
}

// rewriteModelInRequest 替换JSON请求体中的模型名称
// 参数：
//   - body: 原始请求体字节
//   - newModel: 新的模型名称
// 返回：修改后的请求体字节
func rewriteModelInRequest(body []byte, newModel string) []byte {
	// 检查请求体中是否存在"model"字段
	if !gjson.GetBytes(body, "model").Exists() {
		return body
	}
	// 使用sjson库设置新的模型名称
	result, err := sjson.SetBytes(body, "model", newModel)
	if err != nil {
		log.Warnf("amp model mapping: failed to rewrite model in request body: %v", err)
		return body
	}
	return result
}

// extractModelFromRequest 尝试从各种请求格式中提取模型名称
// 支持的格式：
//   - JSON请求体中的"model"字段（OpenAI、Claude等）
//   - URL路径中的模型名称（Gemini）
// 参数：
//   - body: 请求体字节
//   - c: Gin上下文
// 返回：提取的模型名称，如果无法提取则返回空字符串
func extractModelFromRequest(body []byte, c *gin.Context) string {
	// 首先尝试从JSON请求体解析（OpenAI、Claude等）
	// 检查常见的模型字段名
	if result := gjson.GetBytes(body, "model"); result.Exists() && result.Type == gjson.String {
		return result.String()
	}

	// 对于Gemini请求，模型在URL路径中
	// 标准格式：/models/{model}:generateContent -> :action参数
	if action := c.Param("action"); action != "" {
		// 按冒号分割以获取模型名称（例如"gemini-pro:generateContent" -> "gemini-pro"）
		parts := strings.Split(action, ":")
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
	}

	// AMP CLI格式：/publishers/google/models/{model}:method -> *path参数
	// 示例：/publishers/google/models/gemini-3-pro-preview:streamGenerateContent
	if path := c.Param("path"); path != "" {
		// 查找/models/{model}:method模式
		if idx := strings.Index(path, "/models/"); idx >= 0 {
			modelPart := path[idx+8:] // 跳过"/models/"
			// 按冒号分割以获取模型名称
			if colonIdx := strings.Index(modelPart, ":"); colonIdx > 0 {
				return modelPart[:colonIdx]
			}
		}
	}

	return ""
}

