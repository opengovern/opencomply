// @ts-nocheck
import {
    Button,
    Card,
    Col,
    Flex,
    Grid,
    TableBody,
    TableCell,
    TableHead,
    TableHeaderCell,
    TableRow,
    Text,
    Title,
} from '@tremor/react'
import { ChevronRightIcon } from '@heroicons/react/24/solid'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
    BookOpenIcon,
    CheckCircleIcon,
    CodeBracketIcon,
    Cog8ToothIcon,
    CommandLineIcon,
    XCircleIcon,
    ChevronDownIcon,
    ChevronUpIcon,
} from '@heroicons/react/24/outline'
import { useEffect, useState } from 'react'
import MarkdownPreview from '@uiw/react-markdown-preview'
import Pagination from '@cloudscape-design/components/pagination'
import DateRangePicker from '@cloudscape-design/components/date-range-picker'

import { useAtomValue } from 'jotai'
import { useComplianceApiV1BenchmarksControlsDetail } from '../../../../../api/compliance.gen'
import Spinner from '../../../../../components/Spinner'
import { numberDisplay } from '../../../../../utilities/numericDisplay'
import DrawerPanel from '../../../../../components/DrawerPanel'
import AnimatedAccordion from '../../../../../components/AnimatedAccordion'
import { searchAtom } from '../../../../../utilities/urlstate'
import {
    GithubComKaytuIoKaytuEnginePkgComplianceApiBenchmarkControlSummary,
    GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus,
    GithubComKaytuIoKaytuEnginePkgControlApiListV2,
    GithubComKaytuIoKaytuEnginePkgControlApiListV2ResponseItem,
} from '../../../../../api/api'
import SideNavigation from '@cloudscape-design/components/side-navigation'
import { Api } from '../../../../../api/api'
import AxiosAPI from '../../../../../api/ApiConfig'
import Table from '@cloudscape-design/components/table'
import Box from '@cloudscape-design/components/box'
import SpaceBetween from '@cloudscape-design/components/space-between'
import TextFilter from '@cloudscape-design/components/text-filter'
import Header from '@cloudscape-design/components/header'
import Badge from '@cloudscape-design/components/badge'
import KButton from '@cloudscape-design/components/button'
import axios from 'axios'
import ReactEChartsCore from 'echarts-for-react/lib/core'
import * as echarts from 'echarts/core'
import {LineChart} from 'echarts/charts'
import {
    BreadcrumbGroup,
    Link,
    PropertyFilter,
} from '@cloudscape-design/components'

interface IPolicies {
    id: string | undefined
    assignments: number
    enable?: boolean
}

export const activeBadge = (status: boolean) => {
    if (status) {
        return (
            <Flex className="w-fit gap-1.5">
                <CheckCircleIcon className="h-4 text-emerald-500" />
                <Text>Active</Text>
            </Flex>
        )
    }
    return (
        <Flex className="w-fit gap-1.5">
            <XCircleIcon className="h-4 text-rose-600" />
            <Text>Inactive</Text>
        </Flex>
    )
}

export const statusBadge = (
    status:
        | GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus
        | undefined
) => {
    if (
        status ===
        GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusPassed
    ) {
        return (
            <Flex className="w-fit gap-1.5">
                <CheckCircleIcon className="h-4 text-emerald-500" />
                <Text>Passed</Text>
            </Flex>
        )
    }
    return (
        <Flex className="w-fit gap-1.5">
            <XCircleIcon className="h-4 text-rose-600" />
            <Text>Failed</Text>
        </Flex>
    )
}

export const treeRows = (
    json:
        | GithubComKaytuIoKaytuEnginePkgComplianceApiBenchmarkControlSummary
        | undefined
) => {
    let arr: any = []
    if (json) {
        if (json.control !== null && json.control !== undefined) {
            for (let i = 0; i < json.control.length; i += 1) {
                let obj = {}
                obj = {
                    parentName: json?.benchmark?.title,
                    ...json.control[i].control,
                    ...json.control[i],
                }
                arr.push(obj)
            }
        }
        if (json.children !== null && json.children !== undefined) {
            for (let i = 0; i < json.children.length; i += 1) {
                const res = treeRows(json.children[i])
                arr = arr.concat(res)
            }
        }
    }

    return arr
}

export const groupBy = (input: any[], key: string) => {
    return input.reduce((acc, currentValue) => {
        const groupKey = currentValue[key]
        if (!acc[groupKey]) {
            acc[groupKey] = []
        }
        acc[groupKey].push(currentValue)
        return acc
    }, {})
}

export const countControls = (
    v:
        | GithubComKaytuIoKaytuEnginePkgComplianceApiBenchmarkControlSummary
        | undefined
) => {
    const countChildren = v?.children
        ?.map((i) => countControls(i))
        .reduce((prev, curr) => prev + curr, 0)
    const total: number = (countChildren || 0) + (v?.control?.length || 0)
    return total
}

export default function Controls({
    id,
    assignments,
    enable,
    accounts,
}: IPolicies) {
    const { response: controls, isLoading } =
        useComplianceApiV1BenchmarksControlsDetail(String(id))
    const [page, setPage] = useState<number>(1)
    const [rows, setRows] = useState<
        GithubComKaytuIoKaytuEnginePkgControlApiListV2ResponseItem[]
    >([])
    const navigate = useNavigate()
    const searchParams = useAtomValue(searchAtom)
    const [benchmarkId, setBenchmarkId] = useState(id)
    const [loading, setLoading] = useState(true)

    const [doc, setDoc] = useState('')
    const [docTitle, setDocTitle] = useState('')
    const [openAllControls, setOpenAllControls] = useState(false)
    const [listofTables, setListOfTables] = useState([])
    const [totalPage, setTotalPage] = useState<number>(0)
    const [totalCount, setTotalCount] = useState<number>(0)
    const [query, setQuery] =
        useState<GithubComKaytuIoKaytuEnginePkgControlApiListV2>()
    const [queries, setQueries] = useState({
        tokens: [],
        operation: 'and',
    })

    const [tree, setTree] = useState()
    const [selected, setSelected] = useState()
    const [selectedBread, setSelectedBread] = useState()
    const [treePage, setTreePage] = useState(0)
    const [treeTotal, setTreeTotal] = useState(0)
    const [treeTotalPages, setTreeTotalPages] = useState(0)


    const [filters, setFilters] = useState([
        {
            propertyKey: 'severity',
            value: 'High',
        },
        {
            propertyKey: 'severity',
            value: 'Critical',
        },
        {
            propertyKey: 'severity',
            value: 'Low',
        },
        {
            propertyKey: 'severity',
            value: 'Medium',
        },
    ])
    const [filterOption, setFilterOptions] = useState([
        {
            key: 'severity',
            operators: ['='],
            propertyLabel: `Severity`,
            groupValuesLabel: `Severity values`,
        },
        // {
        //     key: 'severity',
        //     operators: ['='],
        //     propertyLabel: `Exclude Inactive Integration'`,
        //     groupValuesLabel: `Exclude Inactive Integration' values`,
        // },
    ])
    const [sort, setSort] = useState('incidents')
    const [sortOrder, setSortOrder] = useState(true)

    const navigateToInsightsDetails = (id: string) => {
        navigate(`${id}?${searchParams}`)
    }
    const toggleOpen = () => {
        setOpenAllControls(!openAllControls)
    }

    const countBenchmarks = (
        v:
            | GithubComKaytuIoKaytuEnginePkgComplianceApiBenchmarkControlSummary
            | undefined
    ) => {
        const countChildren = v?.children
            ?.map((i) => countBenchmarks(i))
            .reduce((prev, curr) => prev + curr, 0)
        const total: number = (countChildren || 0) + (v?.children?.length || 0)
        return total
    }
    const truncate = (text: string | undefined) => {
        if (text) {
            return text.length > 30 ? text.substring(0, 30) + '...' : text
        }
    }
    const GetControls = (flag: boolean, id: string | undefined) => {
        setLoading(true)
        const api = new Api()
        api.instance = AxiosAPI
        //   const benchmarks = category
        //   const temp = []
        //   temp.push(`aws_score_${benchmarks}`)
        //   temp.push(`azure_score_${benchmarks}`)

        let body = {
            // list_of_tables: listofTables,
            severity: query?.severity,
            root_benchmark: flag ? [id] : [benchmarkId],
            finding_summary: enable,
            cursor: page,
            per_page: 10,
            sort_by: sort,
            sort_order: sortOrder ? 'desc' : 'asc',
        }
        if (listofTables.length == 0) {
            // @ts-ignore
            delete body['list_of_tables']
        }
        api.compliance
            // @ts-ignore
            .apiV2ControlList(body)
            .then((resp) => {
                setTotalPage(Math.ceil(resp.data.total_count / 10))
                setTotalCount(resp.data.total_count)
                if (resp.data.items) {
                    setRows(resp.data.items)
                }

                setLoading(false)
            })
            .catch((err) => {
                setLoading(false)
                setRows([])
            })
    }
    const GetTree = () => {
        let url = ''
        if (window.location.origin === 'http://localhost:3000') {
            url = window.__RUNTIME_CONFIG__.REACT_APP_BASE_URL
        } else {
            url = window.location.origin
        }
        // @ts-ignore
        const token = JSON.parse(localStorage.getItem('openg_auth')).token

        const config = {
            headers: {
                Authorization: `Bearer ${token}`,
            },
        }
        axios
            .get(
                `${url}/main/compliance/api/v3/benchmarks/${benchmarkId}/nested`,
                config
            )
            .then((res) => {
                const temp = []

                if (res.data.children) {
                    // temp.push({
                    //     text: res.data.title,
                    //     href: res.data.id,
                    //     type: 'link',
                    // })
                    res.data.children.map((item) => {
                        let childs = {
                            text: truncate(item.title),
                            href: item.id,
                            type: item.children
                                ? 'expandable-link-group'
                                : 'link',
                        }
                        if (item.children && item.children.length > 0) {
                            let child_item = []
                            item.children.map((sub_item) => {
                                child_item.push({
                                    text: truncate(sub_item.title),
                                    href: sub_item.id,
                                    type: 'link',
                                    parentId: item.id,
                                    parentTitle: item.title,
                                })
                            })
                            childs['items'] = child_item
                        }
                        temp.push(childs)
                    })
                  
                    // setTree(temp)
                } else {
                    // temp.push({
                    //     text: res.data.title,
                    //     href: res.data.id,
                    //     type: 'link',
                    // })
                }
                  setTreeTotalPages(Math.ceil(temp.length / 12))
                  setTreeTotal(temp.length)
                  setTreePage(0)

                setTree(temp)
            })
            .catch((err) => {
                console.log(err)
            })
    }
    useEffect(() => {
        GetControls(false)
        GetTree()

    }, [])
    useEffect(() => {
        if (selected) {
            setPage(0)
            GetControls(true, selected)
        }
    }, [selected])
    useEffect(() => {
        if (selected) {
            GetControls(true, selected)
        } else {
            GetControls(false)
        }
    }, [page])
    useEffect(() => {
        if (selected) {
            GetControls(true, selected)
        } else {
            GetControls(false)
        }
    }, [sort, sortOrder])
    useEffect(() => {
        let temp = {}

        queries?.tokenGroups?.map((item) => {
            // @ts-ignore
            if (temp[item.propertyKey] && temp[item.propertyKey].length > 0) {
                temp[item.propertyKey].push(item.value.toLowerCase())
            } else {
                temp[item.propertyKey] = []
                temp[item.propertyKey].push(item.value.toLowerCase())
            }
        })
        setQuery(temp)
    }, [queries])

    useEffect(() => {
        if (selected) {
            GetControls(true, selected)
        } else {
            GetControls(false)
        }
    }, [query])
    return (
        <Grid numItems={12} className="gap-4">
            <Col numColSpan={12}>
                <BreadcrumbGroup
                    onClick={(event) => {
                        event.preventDefault()
                        setSelected(event.detail.href)
                    }}
                    items={selectedBread}
                    ariaLabel="Breadcrumbs"
                />
            </Col>
            {tree && tree.length > 0 && (
                <Col numColSpan={3}>
                    <Flex
                        className="bg-white  w-full border-solid border-2 h-[550px]    rounded-xl gap-1 "
                        flexDirection="col"
                    >
                        <>
                            <SideNavigation
                                className="w-full scroll  h-[550px] overflow-scroll p-4 pb-0"
                                activeHref={selected}
                                virtualScroll
                                header={{
                                    href: benchmarkId,
                                    text: controls?.benchmark?.title,
                                }}
                                onFollow={(event) => {
                                    event.preventDefault()
                                    setSelected(event.detail.href)
                                    const temp = []

                                    if (event.detail.parentId) {
                                        temp.push({
                                            text: controls?.benchmark?.title,
                                            href: id,
                                        })
                                        temp.push({
                                            text: event.detail.parentTitle,
                                            href: event.detail.parentId,
                                        })
                                        temp.push({
                                            text: event.detail.text,
                                            href: event.detail.href,
                                        })
                                    } else {
                                        temp.push({
                                            text: controls?.benchmark?.title,
                                            href: id,
                                        })
                                        if (
                                            event.detail.text !==
                                            controls?.benchmark?.title
                                        ) {
                                            temp.push({
                                                text: event.detail.text,
                                                href: event.detail.href,
                                            })
                                        }
                                    }
                                    setSelectedBread(temp)
                                }}
                                items={tree?.slice(
                                    treePage * 12,
                                    (treePage + 1) * 12
                                )}
                            />
                        </>
                        {treeTotalPages > 1 && (
                            <>
                                <Pagination
                                    className="pb-2"
                                    currentPageIndex={treePage + 1}
                                    pagesCount={treeTotalPages}
                                    onChange={({ detail }) =>
                                        setTreePage(detail.currentPageIndex - 1)
                                    }
                                />
                            </>
                        )}
                    </Flex>
                </Col>
            )}
            <Col numColSpan={tree && tree.length > 0 ? 9 : 12}>
                {' '}
                <Flex className="flex flex-col  min-h-[550px] ">
                    <Table
                        className="p-3   min-h-[550px]"
                        // resizableColumns
                        renderAriaLive={({
                            firstIndex,
                            lastIndex,
                            totalItemsCount,
                        }) =>
                            `Displaying items ${firstIndex} to ${lastIndex} of ${totalItemsCount}`
                        }
                        onSortingChange={(event) => {
                            setSort(event.detail.sortingColumn.sortingField)
                            setSortOrder(!sortOrder)
                        }}
                        sortingColumn={sort}
                        sortingDescending={sortOrder}
                        // sortingDescending={sortOrder == 'desc' ? true : false}
                        columnDefinitions={[
                            {
                                id: 'id',
                                header: 'ID',
                                cell: (item) => item.id,
                                sortingField: 'id',
                                isRowHeader: true,
                            },
                            {
                                id: 'title',
                                header: 'Title',
                                cell: (item) => (
                                    <Link
                                        href={`${window.location}/${item.id}`}
                                        target='__blank'

                                        // onClick={() => {
                                        //     navigateToInsightsDetails(item.id)
                                        // }}
                                    >
                                        {item.title}
                                    </Link>
                                ),
                                sortingField: 'title',
                                // minWidth: 400,
                                maxWidth: 200,
                            },
                            {
                                id: 'connector',
                                header: 'Connector',
                                cell: (item) => item.connector,
                            },
                            {
                                id: 'query',
                                header: 'Primary Table',
                                cell: (item) => item?.query?.primary_table,
                            },
                            {
                                id: 'severity',
                                header: 'Severity',
                                sortingField: 'severity',
                                cell: (item) => (
                                    <Badge
                                        // @ts-ignore
                                        color={`severity-${item.severity}`}
                                    >
                                        {item.severity.charAt(0).toUpperCase() +
                                            item.severity.slice(1)}
                                    </Badge>
                                ),
                                maxWidth: 100,
                            },
                            {
                                id: 'query.parameters',
                                header: 'Has Parametrs',
                                cell: (item) => (
                                    // @ts-ignore
                                    <>
                                        {item.query?.parameters.length > 0
                                            ? 'True'
                                            : 'False'}
                                    </>
                                ),
                            },
                            {
                                id: 'incidents',
                                header: 'Incidents',
                                // sortingField: 'incidents',

                                cell: (item) => (
                                    // @ts-ignore
                                    <>
                                        {/**@ts-ignore */}
                                        {item?.findings_summary?.incident_count
                                            ? item?.findings_summary
                                                  ?.incident_count
                                            : 0}
                                    </>
                                ),
                                // minWidth: 50,
                                maxWidth: 100,
                            },
                            {
                                id: 'passing_resources',
                                header: 'Non Incidents ',

                                cell: (item) => (
                                    // @ts-ignore
                                    <>
                                        {item?.findings_summary
                                            ?.non_incident_count
                                            ? item?.findings_summary
                                                  ?.non_incident_count
                                            : 0}
                                    </>
                                ),
                                maxWidth: 100,
                            },
                            {
                                id: 'noncompliant_resources',
                                header: 'Non-Compliant Resources',
                                sortingField: 'noncompliant_resources',

                                cell: (item) => (
                                    // @ts-ignore
                                    <>
                                        {item?.findings_summary
                                            ?.noncompliant_resources
                                            ? item?.findings_summary
                                                  ?.noncompliant_resources
                                            : 0}
                                    </>
                                ),
                                maxWidth: 100,
                            },
                            {
                                id: 'waste',
                                header: 'Waste',
                                cell: (item) => (
                                    // @ts-ignore
                                    <>
                                        {item?.findings_summary
                                            ?.cost_optimization
                                            ? item?.findings_summary
                                                  ?.cost_optimization
                                            : 0}
                                    </>
                                ),
                                maxWidth: 100,
                            },
                            {
                                id: 'action',
                                header: 'Action',
                                cell: (item) => (
                                    // @ts-ignore
                                    <KButton
                                        onClick={() => {
                                            navigateToInsightsDetails(item.id)
                                        }}
                                        variant="inline-link"
                                        ariaLabel={`Open Detail`}
                                    >
                                        Open
                                    </KButton>
                                ),
                            },
                        ]}
                        columnDisplay={[
                            { id: 'id', visible: false },
                            { id: 'title', visible: true },
                            { id: 'connector', visible: false },
                            { id: 'query', visible: false },
                            { id: 'severity', visible: true },
                            { id: 'incidents', visible: false },
                            { id: 'passing_resources', visible: false },
                            {
                                id: 'noncompliant_resources',
                                visible: enable,
                            },
                            {
                                id: 'waste',
                                visible:
                                    (benchmarkId == 'sre_efficiency' ||
                                        selected == 'sre_efficiency') &&
                                    enable
                                        ? true
                                        : false,
                            },

                            // { id: 'action', visible: true },
                        ]}
                        enableKeyboardNavigation
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
                            <PropertyFilter
                                // @ts-ignore
                                query={queries}
                                // @ts-ignore
                                onChange={({ detail }) => {
                                    // @ts-ignore
                                    setQueries(detail)
                                }}
                                // countText="5 matches"
                                enableTokenGroups
                                expandToViewport
                                filteringAriaLabel="Control Categories"
                                // @ts-ignore
                                // filteringOptions={filters}
                                filteringPlaceholder="Control Categories"
                                // @ts-ignore
                                filteringOptions={filters}
                                filteringProperties={filterOption}
                                // filteringProperties={
                                //     filterOption
                                // }
                            />
                        }
                        header={
                            <Header className="w-full">
                                Controls{' '}
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
                </Flex>
            </Col>
        </Grid>
    )
}
