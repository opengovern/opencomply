import { useEffect, useMemo, useState } from 'react'
import { useAtomValue, useSetAtom } from 'jotai/index'
import {
    ICellRendererParams,
    RowClickedEvent,
    ValueFormatterParams,
    IServerSideGetRowsParams,
} from 'ag-grid-community'
import { Card, Flex, Text, Title } from '@tremor/react'
import { ExclamationCircleIcon } from '@heroicons/react/24/outline'
import Table, { IColumn } from '../../../../../../components/Table'
import { isDemoAtom, notificationAtom } from '../../../../../../store'
import {
    Api,
    GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus,
    GithubComKaytuIoKaytuEnginePkgComplianceApiResourceFinding,
} from '../../../../../../api/api'
import AxiosAPI from '../../../../../../api/ApiConfig'
import { statusBadge } from '../../../index'
import { dateTimeDisplay } from '../../../../../../utilities/dateDisplay'
import { getConnectorIcon } from '../../../../../../components/Cards/ConnectorCard'
import ResourceFindingDetail from '../../../../Findings/ResourceFindingDetail'
import KTable from '@cloudscape-design/components/table'
import Box from '@cloudscape-design/components/box'
import SpaceBetween from '@cloudscape-design/components/space-between'
import Badge from '@cloudscape-design/components/badge'
import {
    BreadcrumbGroup,
    Header,
    Link,
    Pagination,
    PropertyFilter,
} from '@cloudscape-design/components'
import { AppLayout, SplitPanel } from '@cloudscape-design/components'
let sortKey: any[] = []

interface IImpactedResources {
    controlId: string
    conformanceFilter?: GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus[]
    linkPrefix?: string
    isCostOptimization?: boolean
}

const columns = (
    controlID: string,
    isDemo: boolean,
    isCostOptimization: boolean
) => {
    const temp: IColumn<
        GithubComKaytuIoKaytuEnginePkgComplianceApiResourceFinding,
        any
    >[] = [
        {
            field: 'resourceName',
            headerName: 'Resource name',
            hide: false,
            type: 'string',
            enableRowGroup: true,
            sortable: false,
            filter: true,
            resizable: true,
            flex: 1,
            cellRenderer: (
                param: ICellRendererParams<
                    GithubComKaytuIoKaytuEnginePkgComplianceApiResourceFinding,
                    any
                >
            ) => (
                <Flex flexDirection="col" alignItems="start">
                    <Text className="text-gray-800">
                        {param.data?.resourceName ||
                            (param.data?.findings?.at(0)?.stateActive === false
                                ? 'Resource deleted'
                                : '')}
                    </Text>
                    <Text className={isDemo ? 'blur-sm' : ''}>
                        {param.data?.kaytuResourceID}
                    </Text>
                </Flex>
            ),
        },
        {
            field: 'resourceType',
            headerName: 'Resource type',
            type: 'string',
            enableRowGroup: true,
            sortable: true,
            hide: true,
            filter: true,
            resizable: true,
            flex: 1,
            cellRenderer: (
                param: ICellRendererParams<
                    GithubComKaytuIoKaytuEnginePkgComplianceApiResourceFinding,
                    any
                >
            ) => (
                <Flex flexDirection="col" alignItems="start">
                    <Text className="text-gray-800">
                        {param.data?.resourceTypeLabel}
                    </Text>
                    <Text>{param.data?.resourceType}</Text>
                </Flex>
            ),
        },
        // {
        //     field: 'benchmarkID',
        //     headerName: 'Benchmark',
        //     type: 'string',
        //     enableRowGroup: true,
        //     sortable: false,
        //     hide: true,
        //     filter: true,
        //     resizable: true,
        //     flex: 1,
        //     cellRenderer: (param: ICellRendererParams) => (
        //         <Flex flexDirection="col" alignItems="start">
        //             <Text className="text-gray-800">
        //                 {param.data.parentBenchmarkNames[0]}
        //             </Text>
        //             <Text>{param.value}</Text>
        //         </Flex>
        //     ),
        // },
        {
            field: 'providerConnectionName',
            headerName: 'Account',
            type: 'string',
            enableRowGroup: true,
            hide: false,
            sortable: false,
            filter: true,
            resizable: true,
            flex: 1,
            cellRenderer: (
                param: ICellRendererParams<
                    GithubComKaytuIoKaytuEnginePkgComplianceApiResourceFinding,
                    any
                >
            ) => (
                <Flex flexDirection="row" justifyContent="start">
                    {getConnectorIcon(param.data?.connector, '-ml-2 mr-2')}
                    <Flex
                        flexDirection="col"
                        alignItems="start"
                        className={isDemo ? 'blur-sm' : ''}
                    >
                        <Text className="text-gray-800">
                            {param.data?.providerConnectionName}
                        </Text>
                        <Text>{param.data?.providerConnectionID}</Text>
                    </Flex>
                </Flex>
            ),
        },
        {
            field: 'connectionID',
            headerName: 'OpenGovernance connection ID',
            type: 'string',
            hide: true,
            enableRowGroup: true,
            sortable: false,
            filter: true,
            resizable: true,
            flex: 1,
        },
        // {
        //     field: 'stateActive',
        //     headerName: 'State',
        //     type: 'string',
        //     sortable: true,
        //     filter: true,
        //     hide: false,
        //     resizable: true,
        //     flex: 1,
        //     cellRenderer: (param: ValueFormatterParams) => (
        //         <Flex className="h-full">{activeBadge(param.value)}</Flex>
        //     ),
        // },
        {
            field: 'failedCount',
            headerName: 'Conformance status',
            type: 'string',
            sortable: false,
            filter: true,
            hide: false,
            resizable: true,
            width: 160,
            cellRenderer: (
                param: ICellRendererParams<GithubComKaytuIoKaytuEnginePkgComplianceApiResourceFinding>
            ) => {
                return (
                    <Flex className="h-full">
                        {statusBadge(
                            param.data?.findings
                                ?.filter((f) => f.controlID === controlID)
                                .sort((a, b) => {
                                    if (
                                        (a.evaluatedAt || 0) ===
                                        (b.evaluatedAt || 0)
                                    ) {
                                        return 0
                                    }
                                    return (a.evaluatedAt || 0) <
                                        (b.evaluatedAt || 0)
                                        ? 1
                                        : -1
                                })
                                .map((f) => f.conformanceStatus)
                                .at(0)
                        )}
                    </Flex>
                )
            },
        },
        {
            field: 'evaluatedAt',
            headerName: 'Last checked',
            type: 'datetime',
            sortable: false,
            filter: true,
            resizable: true,
            width: 200,
            valueFormatter: (param: ValueFormatterParams) => {
                return param.value ? dateTimeDisplay(param.value) : ''
            },
            hide: false,
        },
    ]

    if (isCostOptimization) {
        temp.push({
            field: 'findings',
            headerName: 'Potential Savings',
            type: 'number',
            sortable: true,
            filter: true,
            hide: false,
            resizable: true,
            width: 150,
            cellRenderer: (
                param: ICellRendererParams<GithubComKaytuIoKaytuEnginePkgComplianceApiResourceFinding>
            ) => {
                return (
                    <Flex className="h-full">
                        $
                        {param.data?.findings
                            ?.filter((f) => f.controlID === controlID)
                            .sort((a, b) => {
                                if (
                                    (a.evaluatedAt || 0) ===
                                    (b.evaluatedAt || 0)
                                ) {
                                    return 0
                                }
                                return (a.evaluatedAt || 0) <
                                    (b.evaluatedAt || 0)
                                    ? 1
                                    : -1
                            })
                            .map((f) => f.costOptimization || 0)
                            .at(0)}
                    </Flex>
                )
            },
        })
    }
    return temp
}

export default function ImpactedResources({
    controlId,
    conformanceFilter,
    linkPrefix,
    isCostOptimization,
}: IImpactedResources) {
    const isDemo = useAtomValue(isDemoAtom)
    const setNotification = useSetAtom(notificationAtom)

    const [open, setOpen] = useState(false)
    const [finding, setFinding] = useState<
        GithubComKaytuIoKaytuEnginePkgComplianceApiResourceFinding | undefined
    >(undefined)
    const [error, setError] = useState('')
    const [loading, setLoading] = useState(false)
 const [rows, setRows] =
     useState<GithubComKaytuIoKaytuEnginePkgComplianceApiResourceFinding[]>()
 const [page, setPage] = useState(1)
 const [totalCount, setTotalCount] = useState(0)
 const [totalPage, setTotalPage] = useState(0)

    // const ssr = () => {
    //     return {
    //         getRows: (params: IServerSideGetRowsParams) => {
    //             const api = new Api()
    //             api.instance = AxiosAPI
    //             let sort = params.request.sortModel.length
    //                 ? [
    //                       {
    //                           [params.request.sortModel[0].colId]:
    //                               params.request.sortModel[0].sort,
    //                       },
    //                   ]
    //                 : []

    //             if (
    //                 params.request.sortModel.length &&
    //                 params.request.sortModel[0].colId === 'failedCount'
    //             ) {
    //                 sort = [
    //                     {
    //                         conformanceStatus: params.request.sortModel[0].sort,
    //                     },
    //                 ]
    //             }
    //             api.compliance
    //                 .apiV1ResourceFindingsCreate({
    //                     filters: {
    //                         controlID: [controlId || ''],
    //                         conformanceStatus:
    //                             conformanceFilter === undefined
    //                                 ? [
    //                                       GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusPassed,
    //                                       GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusFailed,
    //                                   ]
    //                                 : conformanceFilter,
    //                     },
    //                     sort,
    //                     limit: 100,
    //                     afterSortKey:
    //                         params.request.startRow === 0 || sortKey.length < 1
    //                             ? []
    //                             : sortKey,
    //                 })
    //                 .then((resp) => {
    //                     params.success({
    //                         rowData: resp.data.resourceFindings || [],
    //                         rowCount: resp.data.totalCount || 0,
    //                     })

    //                     console.log('count:', resp.data.totalCount)

    //                     sortKey =
    //                         resp.data.resourceFindings?.at(
    //                             (resp.data.resourceFindings?.length || 0) - 1
    //                         )?.sortKey || []
    //                 })
    //                 .catch((err) => {
    //                     console.log('err:', err)
    //                     if (
    //                         err.message !==
    //                         "Cannot read properties of null (reading 'NaN')"
    //                     ) {
    //                         setError(err.message)
    //                     }
    //                     params.fail()
    //                 })
    //         },
    //     }
    // }
  const GetRows = () => {
      setLoading(true)
      const api = new Api()
      api.instance = AxiosAPI
      
      api.compliance
          .apiV1ResourceFindingsCreate({
              filters: {
                  controlID: [controlId || ''],
                  conformanceStatus:
                      conformanceFilter === undefined
                          ? [
                                GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusPassed,
                                GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusFailed,
                            ]
                          : conformanceFilter,
                // @ts-ignore
                connectionGroup: ['healthy']
              },
              // sort: [],
              limit: 15,
              // @ts-ignore
              afterSortKey: page == 1 ? [] : rows[rows?.length - 1].sortKey,

              // afterSortKey:
              //    [],
          })
          .then((resp) => {
              setLoading(false)
                if (resp.data.resourceFindings){
                    setRows(resp.data.resourceFindings)

                }
                else{
                    setRows([])
                }
              // @ts-ignore

              setTotalPage(Math.ceil(resp.data.totalCount / 15))
              // @ts-ignore

              setTotalCount(resp.data.totalCount)
              // @ts-ignore
              // sortKey =
              //     resp.data?.resourceFindings?.at(
              //         (resp.data.resourceFindings?.length || 0) - 1
              //     )?.sortKey || []
          })
          .catch((err) => {
              setLoading(false)
                if (
                    err.message !==
                    "Cannot read properties of null (reading 'NaN')"
                ) {
                    setError(err.message)
                }
              setNotification({
                  text: 'Can not Connect to Server',
                  type: 'warning',
              })
          })
  }
    useEffect(() => {
        GetRows()
    }, [page,conformanceFilter])

    // const serverSideRows = useMemo(() => ssr(), [conformanceFilter])

    return (
        <>
            {error.length > 0 && (
                <Flex className="w-fit mb-3 gap-1">
                    <ExclamationCircleIcon className="text-rose-600 h-5" />
                    <Text color="rose">{error}</Text>
                </Flex>
            )}
            <AppLayout
                toolsOpen={false}
                navigationOpen={false}
                contentType="table"
                toolsHide={true}
                navigationHide={true}
                splitPanelOpen={open}
                onSplitPanelToggle={() => {
                    setOpen(!open)
                    if (open) {
                        setFinding(undefined)
                    }
                }}
                splitPanel={
                    // @ts-ignore
                    <SplitPanel
                        // @ts-ignore
                        header={
                            finding ? (
                                <>
                                    <Flex justifyContent="start">
                                        {getConnectorIcon(finding?.connector)}
                                        <Title className="text-lg font-semibold ml-2 my-1">
                                            {finding?.resourceName}
                                        </Title>
                                    </Flex>
                                </>
                            ) : (
                                'Resource not selected'
                            )
                        }
                    >
                        <ResourceFindingDetail
                            // type="resource"
                            resourceFinding={finding}
                            open={open}
                            showOnlyOneControl={false}
                            onClose={() => setOpen(false)}
                            onRefresh={() => window.location.reload()}
                        />
                    </SplitPanel>
                }
                content={
                    <KTable
                        className="min-h-[450px]"
                        // resizableColumns
                        renderAriaLive={({
                            firstIndex,
                            lastIndex,
                            totalItemsCount,
                        }) =>
                            `Displaying items ${firstIndex} to ${lastIndex} of ${totalItemsCount}`
                        }
                        variant="full-page"
                        onSortingChange={(event) => {
                            // setSort(event.detail.sortingColumn.sortingField)
                            // setSortOrder(!sortOrder)
                        }}
                        // sortingColumn={sort}
                        // sortingDescending={sortOrder}
                        // sortingDescending={sortOrder == 'desc' ? true : false}
                        // @ts-ignore
                        onRowClick={(event) => {
                            const row = event.detail.item
                            if (
                                row?.kaytuResourceID &&
                                row?.kaytuResourceID.length > 0
                            ) {
                                setFinding(row)
                                setOpen(true)
                            } else {
                                setNotification({
                                    text: 'Detail for this finding is currently not available',
                                    type: 'warning',
                                })
                            }
                        }}
                        columnDefinitions={[
                            {
                                id: 'resourceName',
                                header: 'Resource name',
                                cell: (item) => (
                                    <>
                                        <Flex
                                            flexDirection="col"
                                            alignItems="start"
                                            justifyContent="center"
                                            className="h-full"
                                        >
                                            <Text className="text-gray-800">
                                                {item.resourceName}
                                            </Text>
                                            <Text
                                                className={
                                                    isDemo ? 'blur-sm' : ''
                                                }
                                            >
                                                {item.kaytuResourceID}
                                            </Text>
                                        </Flex>
                                    </>
                                ),
                                sortingField: 'id',
                                isRowHeader: true,
                                maxWidth: 200,
                            },
                            {
                                id: 'resourceType',
                                header: 'Resource type',
                                cell: (item) => (
                                    <>
                                        <Flex
                                            flexDirection="col"
                                            alignItems="start"
                                            justifyContent="center"
                                        >
                                            <Text className="text-gray-800">
                                                {item.resourceType}
                                            </Text>
                                            <Text>
                                                {item.resourceTypeLabel}
                                            </Text>
                                        </Flex>
                                    </>
                                ),
                                sortingField: 'title',
                                // minWidth: 400,
                                maxWidth: 200,
                            },
                            {
                                id: 'providerConnectionName',
                                header: 'Cloud account',
                                maxWidth: 100,
                                cell: (item) => (
                                    <>
                                        <Flex
                                            justifyContent="start"
                                            className={`h-full gap-3 group relative ${
                                                isDemo ? 'blur-sm' : ''
                                            }`}
                                        >
                                            {getConnectorIcon(item.connector)}
                                            <Flex
                                                flexDirection="col"
                                                alignItems="start"
                                            >
                                                <Text className="text-gray-800">
                                                    {
                                                        item.providerConnectionName
                                                    }
                                                </Text>
                                                <Text>
                                                    {item.providerConnectionID}
                                                </Text>
                                            </Flex>
                                            <Card className="cursor-pointer absolute w-fit h-fit z-40 right-1 scale-0 transition-all py-1 px-4 group-hover:scale-100">
                                                <Text color="blue">Open</Text>
                                            </Card>
                                        </Flex>
                                    </>
                                ),
                            },
                            {
                                id: 'failedCount',
                                header: 'Conformance status',
                                maxWidth: 100,
                                cell: (item) => (
                                    <>
                                        {' '}
                                        <Flex className="h-full">
                                            {statusBadge(
                                                item?.findings

                                                    ?.filter(
                                                        (f) =>
                                                            f.controlID ===
                                                            controlId
                                                    )
                                                    .sort((a, b) => {
                                                        if (
                                                            (a.evaluatedAt ||
                                                                0) ===
                                                            (b.evaluatedAt || 0)
                                                        ) {
                                                            return 0
                                                        }
                                                        return (a.evaluatedAt ||
                                                            0) <
                                                            (b.evaluatedAt || 0)
                                                            ? 1
                                                            : -1
                                                    })
                                                    .map(
                                                        (f) =>
                                                            f.conformanceStatus
                                                    )
                                                    .at(0)
                                            )}
                                        </Flex>
                                    </>
                                ),
                            },
                            {
                                id: 'totalCount',
                                header: 'Findings',
                                maxWidth: 100,

                                cell: (item) => (
                                    <>
                                        <Flex
                                            flexDirection="col"
                                            alignItems="start"
                                            justifyContent="center"
                                            className="h-full"
                                        >
                                            <Text className="text-gray-800">{`${item.totalCount} issues`}</Text>
                                            <Text>{`${
                                                // @ts-ignore
                                                item.totalCount -
                                                // @ts-ignore
                                                item.failedCount
                                            } passed`}</Text>
                                        </Flex>
                                    </>
                                ),
                            },
                            // {
                            //     id: 'conformanceStatus',
                            //     header: 'Status',
                            //     sortingField: 'severity',
                            //     cell: (item) => (
                            //         <Badge
                            //             // @ts-ignore
                            //             color={`${
                            //                 item.conformanceStatus == 'passed'
                            //                     ? 'green'
                            //                     : 'red'
                            //             }`}
                            //         >
                            //             {item.conformanceStatus}
                            //         </Badge>
                            //     ),
                            //     maxWidth: 100,
                            // },
                            // {
                            //     id: 'severity',
                            //     header: 'Severity',
                            //     sortingField: 'severity',
                            //     cell: (item) => (
                            //         <Badge
                            //             // @ts-ignore
                            //             color={`severity-${item.severity}`}
                            //         >
                            //             {item.severity.charAt(0).toUpperCase() +
                            //                 item.severity.slice(1)}
                            //         </Badge>
                            //     ),
                            //     maxWidth: 100,
                            // },
                            // {
                            //     id: 'evaluatedAt',
                            //     header: 'Last Evaluation',
                            //     cell: (item) => (
                            //         // @ts-ignore
                            //         <>{dateTimeDisplay(item.value)}</>
                            //     ),
                            // },
                        ]}
                        columnDisplay={[
                            { id: 'resourceName', visible: true },
                            { id: 'resourceType', visible: true },
                            { id: 'providerConnectionName', visible: true },
                            // { id: 'totalCount', visible: true },
                            // { id: 'severity', visible: true },
                            // { id: 'evaluatedAt', visible: true },

                            // { id: 'action', visible: true },
                        ]}
                        enableKeyboardNavigation
                        // @ts-ignore
                        items={rows}
                        loading={loading}
                        loadingText="Loading resources"
                        // stickyColumns={{ first: 0, last: 1 }}
                        // stripedRows
                        trackBy="id"
                        empty={
                            <Box
                                margin={{ vertical: 'xs' }}
                                textAlign="center"
                                color="inherit"
                            >
                                <SpaceBetween size="m">
                                    <b>No resources</b>
                                </SpaceBetween>
                            </Box>
                        }
                        filter={
                            ''
                            // <PropertyFilter
                            //     // @ts-ignore
                            //     query={undefined}
                            //     // @ts-ignore
                            //     onChange={({ detail }) => {
                            //         // @ts-ignore
                            //         setQueries(detail)
                            //     }}
                            //     // countText="5 matches"
                            //     enableTokenGroups
                            //     expandToViewport
                            //     filteringAriaLabel="Control Categories"
                            //     // @ts-ignore
                            //     // filteringOptions={filters}
                            //     filteringPlaceholder="Control Categories"
                            //     // @ts-ignore
                            //     filteringOptions={undefined}
                            //     // @ts-ignore

                            //     filteringProperties={undefined}
                            //     // filteringProperties={
                            //     //     filterOption
                            //     // }
                            // />
                        }
                        header={
                            <Header className="w-full">
                                Resources{' '}
                                <span className=" font-medium">
                                    ({totalCount})
                                </span>
                            </Header>
                        }
                        pagination={
                            <Pagination
                                currentPageIndex={page}
                                pagesCount={totalPage}
                                onChange={({ detail }) =>
                                    setPage(detail.currentPageIndex)
                                }
                            />
                        }
                    />
                }
            />
            {/* <ResourceFindingDetail
                resourceFinding={finding}
                controlID={controlId}
                showOnlyOneControl
                open={open}
                onClose={() => setOpen(false)}
                onRefresh={() => window.location.reload()}
                linkPrefix={linkPrefix}
            /> */}
        </>
    )
}
