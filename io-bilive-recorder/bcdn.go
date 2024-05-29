package main

import (
	"context"
	"fmt"
	proxy "github.com/TiyaAnlite/FocotServices/client-http-proxy/api"
	"go.opentelemetry.io/otel/attribute"
	"k8s.io/klog/v2"
	"time"
)

var ISPName = map[string]string{
	"ct": "电信",
	"cm": "移动",
	"cu": "联通",
	"gd": "广电",
	"fx": "教育网",
	"cc": "鹏博士",
	"se": "教育网(北京)",
	"eq": "BiliBili香港",
	"ix": "电信(上海-zhao-1)",
}

// BCdnLookup Lookup BiliCDN domain, return nil when domain not found
func (syncer *DataSyncer) BCdnLookup(domain string) (region *BCdnDomainInfo) {
	syncer.mu.RLock()
	defer syncer.mu.RUnlock()
	if syncer.bcdnRegions != nil {
		region, _ = syncer.bcdnRegions[domain]
	}
	return
}

// BCdnLookupBatch Lookup batch of BiliCDN domain for avoid to require lock each one, return nil which domain not found
func (syncer *DataSyncer) BCdnLookupBatch(domains []string) (regions []*BCdnDomainInfo) {
	regions = make([]*BCdnDomainInfo, len(domains))
	syncer.mu.RLock()
	defer syncer.mu.RUnlock()
	if syncer.bcdnRegions != nil {
		for i, domain := range domains {
			regions[i], _ = syncer.bcdnRegions[domain]
		}
	}
	return
}

// GetBCdn Pull BiliCDN from GitHub via http-proxy
func (syncer *DataSyncer) GetBCdn(proxyNode string) (data *BCdnDnsData, err error) {
	req := proxy.NewRequest(
		proxy.WithRequestHost("raw.githubusercontent.com"),
		proxy.WithRequestPath("/BililiveRecorder/website/main/src/data/cdn/bcdn.json"),
	)
	data, err = proxy.SendTypedRequest[BCdnDnsData](mq, proxyNode, req, 10)
	return
}

// BCdnUpdater BiliCDN lookup updater, using parent worker
func (syncer *DataSyncer) BCdnUpdater(wg *worker, interval int, proxyNode string) {
	wg.Add(1)
	defer wg.Done()
	klog.Infof("BCdn updater start, interval is %d", interval)
	// first fetch
	ctx, firstTrace := wg.Start(wg.Ctx, "BCdnUpdater")
	defer firstTrace.End()
	for {
		if err := syncer.updateBCdn(ctx, wg, proxyNode); err != nil {
			klog.Errorf("[BiliCDN]First fetch failed, retry at 10 second")
			timer := time.NewTimer(time.Second * 10)
			select {
			case <-timer.C:
				continue // Retry at 10 second
			case <-ctx.Done():
				klog.Infof("BCdn updater end at first fetch.")
				return // Ctx end
			}
		} else {
			break
		}
	}
	firstTrace.End()
	// fetch loop
	ticker := time.NewTicker(time.Duration(int64(time.Second) * int64(cfg.BCdnRegionUpdateInterval)))
	for {
		select {
		case <-ticker.C:
			ctx, updateTrace := wg.Start(wg.Ctx, "BCdnUpdater")
			if err := syncer.updateBCdn(ctx, wg, proxyNode); err != nil {
				klog.Errorf("[BiliCDN]Fetch failed")
			}
			updateTrace.End()
		case <-wg.Ctx.Done():
			klog.Infof("BCdn updater end.")
			return
		}
	}
}

func (syncer *DataSyncer) updateBCdn(ctx context.Context, wg *worker, proxyNode string) (err error) {
	var data *BCdnDnsData
	_, cdnTrace := wg.Start(ctx, "requestBCdn")
	cdnTrace.SetAttributes(attribute.String("bilive.httpProxyNode", proxyNode))
	defer cdnTrace.End()
	data, err = syncer.GetBCdn(proxyNode)
	if err != nil {
		klog.Errorf("Get BCdn failed: %s", err.Error())
		cdnTrace.RecordError(err)
		return
	}
	cdnTrace.End()
	if syncer.bcdn != nil && syncer.bcdn.Version == data.Version {
		// newest
		return
	}
	_, parseTrace := wg.Start(ctx, "parseBCdn")
	defer parseTrace.End()
	node := syncer.parseBCdnData(data)
	parseTrace.End()
	syncer.mu.Lock()
	syncer.bcdn = data
	syncer.bcdnRegions = node
	syncer.mu.Unlock()
	return
}

func (syncer *DataSyncer) parseBCdnData(data *BCdnDnsData) (node map[string]*BCdnDomainInfo) {
	node = make(map[string]*BCdnDomainInfo)
	regionTypeCount := len(data.DNSList)
	regionCount := 0
	nodesCount := 0
	for isp, regionList := range data.DNSList {
		for _, region := range regionList {
			regionCount++
			region.ISP = isp
			for domain, ips := range region.Domains {
				nodesCount++
				node[domain] = &BCdnDomainInfo{
					IPs:        ips,
					RegionInfo: region,
				}
			}
		}
	}
	klog.Info(fmt.Sprintf("[BiliCDN]Parsed %d regionTypes, %d regions, %d domains, version: %s", regionTypeCount, regionCount, nodesCount, data.Version))
	return
}
